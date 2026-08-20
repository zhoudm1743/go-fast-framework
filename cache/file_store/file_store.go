package fileStore

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zhoudm1743/go-fast-framework/contracts"
)

// 文件缓存存储实现（类似 ThinkPHP file 驱动）。
//
// 存储格式：
//   - key 经 md5 哈希作为文件名，前 2 位做二级目录分片，防止路径穿越与单目录文件过多
//   - 文件内容 = 首行过期时间戳（UnixNano，0 表示永不过期）+ "\n" + 值体
//   - 值体复用 redis store 的 "b:"（[]byte 字节保真）/ "j:"（JSON）前缀约定
//   - 写入采用同目录临时文件 + os.Rename 原子替换，读操作永远读到完整旧/新文件
//
// 并发模型：
//   - 读无锁（单次 ReadFile）；读-改-写（Increment/Hash）按 key 哈希分片加锁
//   - 分布式锁为进程内实现（同 memory store）
//   - Tags 索引落盘至 {dir}/.tags/，进程重启后可继续按 tag 批量失效；
//     多进程/多副本并发写同一目录时 tag 索引与缓存文件一样存在竞态，需注意
const (
	// fileStripeCount 读-改-写操作的分片锁数量。
	fileStripeCount = 64
	// gcWriteInterval 每多少次写操作触发一次全目录 GC 扫描。
	gcWriteInterval = 1000
	// cacheFileExt 缓存文件后缀，GC 只清理该后缀文件。
	cacheFileExt = ".cache"
	// tagDirName tag 索引子目录（不参与 .cache GC 扫描）。
	tagDirName = ".tags"
	// tagFileExt tag 索引文件后缀。
	tagFileExt = ".tag"
)

// FileStore 基于本地文件系统的缓存存储实现。
type FileStore struct {
	dir string

	stripes [fileStripeCount]sync.Mutex

	tags  map[string]map[string]struct{}
	tagMu sync.RWMutex

	locks sync.Map

	stopGC chan struct{}
	writes atomic.Uint64
}

// New 创建 FileStore 并确保缓存目录可写（fail-fast）。
// path 为缓存目录；gcInterval > 0 时启动后台 GC 协程清理过期文件。
func New(path string, gcInterval time.Duration) (*FileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("[GoFast] 缓存目录路径不能为空")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("[GoFast] 解析缓存目录 %q 失败: %w", path, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("[GoFast] 创建缓存目录 %s 失败: %w", abs, err)
	}
	// 可写性探测：避免目录只读时静默运行、写缓存时才发现失败
	probe := filepath.Join(abs, ".go-fast-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		return nil, fmt.Errorf("[GoFast] 缓存目录 %s 不可写: %w", abs, err)
	}
	_ = os.Remove(probe)

	s := &FileStore{
		dir:  abs,
		tags: make(map[string]map[string]struct{}),
	}
	if err := s.loadTagIndexes(); err != nil {
		return nil, fmt.Errorf("[GoFast] 加载 tag 索引失败: %w", err)
	}
	s.startGC(gcInterval)
	return s, nil
}

// Stop 停止后台 GC 协程，可安全重复调用。
func (s *FileStore) Stop() {
	if s.stopGC == nil {
		return
	}
	select {
	case <-s.stopGC:
	default:
		close(s.stopGC)
	}
}

func (s *FileStore) startGC(interval time.Duration) {
	if interval <= 0 {
		return
	}
	s.stopGC = make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				s.gc()
			case <-s.stopGC:
				return
			}
		}
	}()
}

// ── 路径与文件格式 ────────────────────────────────────────────────────

// filePath 将 key 转换为缓存文件路径：md5(key) 前 2 位做子目录分片。
// 哈希后文件名固定长度、无路径分隔符，天然防路径穿越。
func (s *FileStore) filePath(key string) string {
	sum := md5.Sum([]byte(key))
	hexStr := hex.EncodeToString(sum[:])
	return filepath.Join(s.dir, hexStr[:2], hexStr[2:]+cacheFileExt)
}

// hashFilePath 哈希结构与普通 key 使用不同命名空间，避免同 key 冲突。
func (s *FileStore) hashFilePath(key string) string {
	return s.filePath("hash:" + key)
}

// stripe 返回 key 对应的分片锁，用于读-改-写操作串行化。
func (s *FileStore) stripe(key string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return &s.stripes[h.Sum32()%fileStripeCount]
}

// encode 将值序列化为存储字符串，与 redis store 保持一致的前缀约定。
func encode(v any) (string, error) {
	if b, ok := v.([]byte); ok {
		return "b:" + string(b), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return "j:" + string(b), nil
}

// decode 将存储字符串还原为 any。
func decode(s string) any {
	if len(s) >= 2 {
		switch s[:2] {
		case "b:":
			return []byte(s[2:])
		case "j:":
			s = s[2:]
		}
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return s
	}
	return v
}

type fileEntry struct {
	value    any
	expireAt int64
}

// parseFile 解析缓存文件内容。
// 首行能解析为 int64 视为过期时间戳（新格式），否则整个文件视为无过期时间的旧数据。
func parseFile(data []byte) fileEntry {
	if idx := bytes.IndexByte(data, '\n'); idx > 0 {
		if exp, err := strconv.ParseInt(string(data[:idx]), 10, 64); err == nil {
			return fileEntry{value: decode(string(data[idx+1:])), expireAt: exp}
		}
	}
	return fileEntry{value: decode(string(data))}
}

// writeFile 以临时文件 + rename 的方式原子写入缓存文件。
func (s *FileStore) writeFile(path string, expireAt int64, body string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".go-fast-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	header := strconv.FormatInt(expireAt, 10) + "\n"
	if _, err := tmp.WriteString(header); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func (s *FileStore) writeEntry(path string, value any, expireAt int64) error {
	body, err := encode(value)
	if err != nil {
		return err
	}
	return s.writeFile(path, expireAt, body)
}

// readFile 读取并解析缓存文件。
// 文件不存在返回 found=false；文件存在但已过期时删除文件并返回 found=false。
func (s *FileStore) readFile(path string) (fileEntry, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileEntry{}, false, nil
		}
		return fileEntry{}, false, err
	}
	e := parseFile(data)
	if e.expireAt > 0 && time.Now().UnixNano() > e.expireAt {
		_ = os.Remove(path)
		return fileEntry{}, false, nil
	}
	return e, true, nil
}

// headerExpireAt 只读文件头部（最多 32 字节）解析过期时间戳，供 GC 使用，
// 避免 GC 全量读取大缓存文件。
func headerExpireAt(path string) (int64, bool, error) {
	fh, err := os.Open(path)
	if err != nil {
		return 0, false, err
	}
	defer fh.Close()
	buf := make([]byte, 32)
	n, err := fh.Read(buf)
	if err != nil && err != io.EOF {
		return 0, false, err
	}
	buf = buf[:n]
	idx := bytes.IndexByte(buf, '\n')
	if idx <= 0 {
		return 0, false, nil // 旧格式或无头文件：不参与过期判断
	}
	exp, err := strconv.ParseInt(string(buf[:idx]), 10, 64)
	if err != nil {
		return 0, false, nil
	}
	return exp, true, nil
}

// ── 基础 CRUD ─────────────────────────────────────────────────────────

func (s *FileStore) Get(key string, def ...any) any {
	e, found, err := s.readFile(s.filePath(key))
	if err != nil || !found {
		if len(def) > 0 {
			return def[0]
		}
		return nil
	}
	return e.value
}

func (s *FileStore) GetBool(key string, def ...bool) bool {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

func (s *FileStore) GetInt(key string, def ...int) int {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	return toInt(v)
}

func (s *FileStore) GetInt64(key string, def ...int64) int64 {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	n, _ := toInt64(v)
	return n
}

func (s *FileStore) GetFloat64(key string, def ...float64) float64 {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func (s *FileStore) GetString(key string, def ...string) string {
	v := s.Get(key)
	if v == nil {
		if len(def) > 0 {
			return def[0]
		}
		return ""
	}
	if str, ok := v.(string); ok {
		return str
	}
	return fmt.Sprintf("%v", v)
}

func (s *FileStore) Has(key string) bool {
	_, found, err := s.readFile(s.filePath(key))
	return err == nil && found
}

// maybeGC 每 gcWriteInterval 次写操作触发一次 GC 扫描（ThinkPHP 概率清理的确定性版）。
func (s *FileStore) maybeGC() {
	if s.writes.Add(1)%gcWriteInterval == 0 {
		s.gc()
	}
}

func (s *FileStore) Put(key string, value any, ttl time.Duration) error {
	s.maybeGC()
	var expireAt int64
	if ttl > 0 {
		expireAt = time.Now().Add(ttl).UnixNano()
	}
	return s.writeEntry(s.filePath(key), value, expireAt)
}

func (s *FileStore) Forever(key string, value any) error {
	return s.Put(key, value, 0)
}

// Forget 删除缓存并清理进程内 tag 引用。
func (s *FileStore) Forget(key string) error {
	err := os.Remove(s.filePath(key))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	s.untrack(key)
	return nil
}

// Flush 删除整个缓存目录并重建（清空全部缓存）。
func (s *FileStore) Flush() error {
	if err := os.RemoveAll(s.dir); err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	s.tagMu.Lock()
	s.tags = make(map[string]map[string]struct{})
	s.tagMu.Unlock()
	return nil
}

func (s *FileStore) Pull(key string, def ...any) any {
	v := s.Get(key, def...)
	_ = s.Forget(key)
	return v
}

// ── 原子操作 ─────────────────────────────────────────────────────────

func (s *FileStore) Increment(key string, value ...int64) (int64, error) {
	delta := int64(1)
	if len(value) > 0 {
		delta = value[0]
	}
	st := s.stripe(key)
	st.Lock()
	defer st.Unlock()

	path := s.filePath(key)
	e, found, err := s.readFile(path)
	if err != nil {
		return 0, err
	}
	if !found {
		if err := s.writeEntry(path, delta, 0); err != nil {
			return 0, err
		}
		return delta, nil
	}
	cur, ok := toInt64(e.value)
	if !ok {
		return 0, fmt.Errorf("[GoFast] cache: increment non-numeric value for key %q", key)
	}
	n := cur + delta
	// 保留原有过期时间，与 memory store 行为一致
	if err := s.writeEntry(path, n, e.expireAt); err != nil {
		return 0, err
	}
	return n, nil
}

func (s *FileStore) Decrement(key string, value ...int64) (int64, error) {
	delta := int64(1)
	if len(value) > 0 {
		delta = value[0]
	}
	return s.Increment(key, -delta)
}

func (s *FileStore) Remember(key string, ttl time.Duration, callback func() (any, error)) (any, error) {
	if v := s.Get(key); v != nil {
		return v, nil
	}
	v, err := callback()
	if err != nil {
		return nil, err
	}
	if err := s.Put(key, v, ttl); err != nil {
		return nil, err
	}
	return v, nil
}

func (s *FileStore) RememberForever(key string, callback func() (any, error)) (any, error) {
	return s.Remember(key, 0, callback)
}

// ── 批量操作 ─────────────────────────────────────────────────────────

func (s *FileStore) Many(keys []string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, k := range keys {
		result[k] = s.Get(k)
	}
	return result
}

func (s *FileStore) PutMany(values map[string]any, ttl time.Duration) error {
	for k, v := range values {
		if err := s.Put(k, v, ttl); err != nil {
			return err
		}
	}
	return nil
}

// ── 标签分组 ─────────────────────────────────────────────────────────

func (s *FileStore) Tags(tags ...string) contracts.TaggedCache {
	return &fileTaggedCache{store: s, tags: tags}
}

func (s *FileStore) track(key string, tags []string) error {
	s.tagMu.Lock()
	defer s.tagMu.Unlock()
	for _, tag := range tags {
		if s.tags[tag] == nil {
			s.tags[tag] = make(map[string]struct{})
		}
		s.tags[tag][key] = struct{}{}
		if err := s.saveTagIndexLocked(tag); err != nil {
			return err
		}
	}
	return nil
}

// untrack 从所有 tag 组中移除 key（Forget 时同步维护）。
func (s *FileStore) untrack(key string) {
	s.tagMu.Lock()
	defer s.tagMu.Unlock()
	for tag, keys := range s.tags {
		if _, ok := keys[key]; !ok {
			continue
		}
		delete(keys, key)
		_ = s.saveTagIndexLocked(tag)
	}
}

// tagDir 返回 tag 索引目录路径。
func (s *FileStore) tagDir() string {
	return filepath.Join(s.dir, tagDirName)
}

// tagFilePath 将 tag 名映射为索引文件路径（md5 哈希，避免特殊字符与路径穿越）。
func (s *FileStore) tagFilePath(tag string) string {
	sum := md5.Sum([]byte(tag))
	hexStr := hex.EncodeToString(sum[:])
	return filepath.Join(s.tagDir(), hexStr+tagFileExt)
}

type tagIndexFile struct {
	Tag  string   `json:"tag"`
	Keys []string `json:"keys"`
}

// loadTagIndexes 启动时从磁盘加载全部 tag 索引，并剔除已无对应缓存文件的 stale key。
func (s *FileStore) loadTagIndexes() error {
	s.tagMu.Lock()
	defer s.tagMu.Unlock()

	entries, err := os.ReadDir(s.tagDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), tagFileExt) {
			continue
		}
		path := filepath.Join(s.tagDir(), ent.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var idx tagIndexFile
		if err := json.Unmarshal(data, &idx); err != nil {
			return fmt.Errorf("解析 tag 索引 %s 失败: %w", path, err)
		}
		if idx.Tag == "" {
			continue
		}
		keys := make(map[string]struct{}, len(idx.Keys))
		for _, k := range idx.Keys {
			if k == "" {
				continue
			}
			if s.Has(k) {
				keys[k] = struct{}{}
			}
		}
		if len(keys) == 0 {
			_ = os.Remove(path)
			continue
		}
		s.tags[idx.Tag] = keys
		if len(keys) != len(idx.Keys) {
			if err := s.saveTagIndexLocked(idx.Tag); err != nil {
				return err
			}
		}
	}
	return nil
}

// saveTagIndexLocked 将单个 tag 的索引原子写入磁盘；调用方需已持有 tagMu。
func (s *FileStore) saveTagIndexLocked(tag string) error {
	keys := s.tags[tag]
	if len(keys) == 0 {
		path := s.tagFilePath(tag)
		err := os.Remove(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		delete(s.tags, tag)
		return nil
	}
	keyList := make([]string, 0, len(keys))
	for k := range keys {
		keyList = append(keyList, k)
	}
	body, err := json.Marshal(tagIndexFile{Tag: tag, Keys: keyList})
	if err != nil {
		return err
	}
	path := s.tagFilePath(tag)
	if err := os.MkdirAll(s.tagDir(), 0o755); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".go-fast-tag-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// deleteTagIndexLocked 删除 tag 索引文件；调用方需已持有 tagMu。
func (s *FileStore) deleteTagIndexLocked(tag string) error {
	delete(s.tags, tag)
	path := s.tagFilePath(tag)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ── Hash 操作 ────────────────────────────────────────────────────────

// readHash 读取哈希文件：值为 field -> 序列化字符串 的 JSON 映射。
func (s *FileStore) readHash(key string) (map[string]string, error) {
	e, found, err := s.readFile(s.hashFilePath(key))
	if err != nil || !found {
		return make(map[string]string), err
	}
	m, ok := e.value.(map[string]any)
	if !ok {
		return make(map[string]string), nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		if str, ok := v.(string); ok {
			out[k] = str
		}
	}
	return out, nil
}

func (s *FileStore) writeHash(key string, m map[string]string) error {
	return s.writeEntry(s.hashFilePath(key), m, 0)
}

func (s *FileStore) HSet(key, field string, value any) error {
	body, err := encode(value)
	if err != nil {
		return err
	}
	st := s.stripe("hash:" + key)
	st.Lock()
	defer st.Unlock()

	m, err := s.readHash(key)
	if err != nil {
		return err
	}
	m[field] = body
	return s.writeHash(key, m)
}

func (s *FileStore) HGet(key, field string) (any, error) {
	m, err := s.readHash(key)
	if err != nil {
		return nil, err
	}
	body, ok := m[field]
	if !ok {
		return nil, nil
	}
	return decode(body), nil
}

func (s *FileStore) HDel(key string, fields ...string) error {
	st := s.stripe("hash:" + key)
	st.Lock()
	defer st.Unlock()

	m, err := s.readHash(key)
	if err != nil {
		return err
	}
	for _, f := range fields {
		delete(m, f)
	}
	if len(m) == 0 {
		// 空哈希直接删除文件，与 memory store 行为一致
		err := os.Remove(s.hashFilePath(key))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return s.writeHash(key, m)
}

func (s *FileStore) HExists(key, field string) bool {
	m, err := s.readHash(key)
	if err != nil {
		return false
	}
	_, exists := m[field]
	return exists
}

func (s *FileStore) HGetAll(key string) (map[string]any, error) {
	m, err := s.readHash(key)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = decode(v)
	}
	return out, nil
}

func (s *FileStore) HLen(key string) int64 {
	m, err := s.readHash(key)
	if err != nil {
		return 0
	}
	return int64(len(m))
}

func (s *FileStore) HKeys(key string) ([]string, error) {
	m, err := s.readHash(key)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}

// ── 分布式锁（进程内） ──────────────────────────────────────────────

// Lock 获取进程内锁（同 memory store 语义，多进程/多副本不共享）。
func (s *FileStore) Lock(key string, ttl time.Duration) contracts.CacheLock {
	actual, _ := s.locks.LoadOrStore(key, &fileLock{})
	return actual.(*fileLock)
}

// ── GC ───────────────────────────────────────────────────────────────

// gc 全目录扫描并删除已过期的缓存文件。
func (s *FileStore) gc() {
	_ = filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), cacheFileExt) {
			return nil
		}
		exp, ok, err := headerExpireAt(path)
		if err == nil && ok && exp > 0 && time.Now().UnixNano() > exp {
			_ = os.Remove(path)
		}
		return nil
	})
}

// ── 辅助 ─────────────────────────────────────────────────────────────

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), true
	}
	return 0, false
}

// ── TaggedCache ──────────────────────────────────────────────────────

type fileTaggedCache struct {
	store *FileStore
	tags  []string
}

func (t *fileTaggedCache) Get(key string, def ...any) any    { return t.store.Get(key, def...) }
func (t *fileTaggedCache) Has(key string) bool               { return t.store.Has(key) }
func (t *fileTaggedCache) Many(keys []string) map[string]any { return t.store.Many(keys) }

func (t *fileTaggedCache) Put(key string, value any, ttl time.Duration) error {
	if err := t.store.Put(key, value, ttl); err != nil {
		return err
	}
	return t.store.track(key, t.tags)
}

func (t *fileTaggedCache) Forever(key string, value any) error {
	if err := t.store.Forever(key, value); err != nil {
		return err
	}
	return t.store.track(key, t.tags)
}

func (t *fileTaggedCache) Forget(key string) error { return t.store.Forget(key) }

func (t *fileTaggedCache) PutMany(values map[string]any, ttl time.Duration) error {
	if err := t.store.PutMany(values, ttl); err != nil {
		return err
	}
	for k := range values {
		if err := t.store.track(k, t.tags); err != nil {
			return err
		}
	}
	return nil
}

func (t *fileTaggedCache) Increment(key string, value ...int64) (int64, error) {
	n, err := t.store.Increment(key, value...)
	if err != nil {
		return n, err
	}
	return n, t.store.track(key, t.tags)
}

func (t *fileTaggedCache) Decrement(key string, value ...int64) (int64, error) {
	n, err := t.store.Decrement(key, value...)
	if err != nil {
		return n, err
	}
	return n, t.store.track(key, t.tags)
}

func (t *fileTaggedCache) Flush() error {
	t.store.tagMu.Lock()
	keysToDelete := make(map[string]struct{})
	var persistErr error
	for _, tag := range t.tags {
		for k := range t.store.tags[tag] {
			keysToDelete[k] = struct{}{}
		}
		if err := t.store.deleteTagIndexLocked(tag); err != nil && persistErr == nil {
			persistErr = err
		}
	}
	t.store.tagMu.Unlock()
	if persistErr != nil {
		return persistErr
	}

	for k := range keysToDelete {
		if err := t.store.Forget(k); err != nil {
			return err
		}
	}
	return nil
}

// ── 进程内锁 ─────────────────────────────────────────────────────────

type fileLock struct {
	acquired atomic.Int32
}

func (l *fileLock) Acquire() bool {
	return l.acquired.CompareAndSwap(0, 1)
}

func (l *fileLock) Release() bool {
	return l.acquired.CompareAndSwap(1, 0)
}

func (l *fileLock) ForceRelease() bool {
	l.acquired.Store(0)
	return true
}

func (l *fileLock) Block(timeout time.Duration, callback ...func()) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if l.Acquire() {
			for _, cb := range callback {
				cb()
			}
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

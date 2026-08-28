package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
)

// EncryptUtil 加解密工具集。
var EncryptUtil = encryptUtil{}

type encryptUtil struct{}

func (r encryptUtil) AESGCMEncrypt(plaintext, key []byte) (string, error) {
	k := normalizeAESKey(key)
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (r encryptUtil) AESGCMDecrypt(encoded string, key []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	k := normalizeAESKey(key)
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("encrypt: ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, nil)
}

func (r encryptUtil) AESCBCEncrypt(plaintext, key, iv []byte) (string, error) {
	k := normalizeAESKey(key)
	block, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	if len(iv) != aes.BlockSize {
		return "", errors.New("encrypt: invalid iv size")
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	out := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(out, padded)
	return base64.StdEncoding.EncodeToString(out), nil
}

func (r encryptUtil) AESCBCDecrypt(encoded string, key, iv []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	k := normalizeAESKey(key)
	block, err := aes.NewCipher(k)
	if err != nil {
		return nil, err
	}
	if len(iv) != aes.BlockSize {
		return nil, errors.New("encrypt: invalid iv size")
	}
	if len(data)%aes.BlockSize != 0 {
		return nil, errors.New("encrypt: invalid ciphertext")
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	out := make([]byte, len(data))
	mode.CryptBlocks(out, data)
	return pkcs7Unpad(out)
}

func (r encryptUtil) RSAEncrypt(plaintext []byte, publicKeyPEM string) ([]byte, error) {
	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return nil, err
	}
	return rsa.EncryptPKCS1v15(rand.Reader, pub, plaintext)
}

func (r encryptUtil) RSADecrypt(ciphertext []byte, privateKeyPEM string) ([]byte, error) {
	priv, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	return rsa.DecryptPKCS1v15(rand.Reader, priv, ciphertext)
}

func (r encryptUtil) RSASign(data []byte, privateKeyPEM string) ([]byte, error) {
	priv, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	return rsa.SignPKCS1v15(rand.Reader, priv, 0, hash[:])
}

func (r encryptUtil) RSAVerify(data, signature []byte, publicKeyPEM string) error {
	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	hash := sha256.Sum256(data)
	return rsa.VerifyPKCS1v15(pub, 0, hash[:], signature)
}

func normalizeAESKey(key []byte) []byte {
	if len(key) == 16 || len(key) == 24 || len(key) == 32 {
		return key
	}
	sum := sha256.Sum256(key)
	return sum[:]
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padLen := blockSize - len(data)%blockSize
	pad := bytesRepeat(byte(padLen), padLen)
	return append(data, pad...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("encrypt: invalid padding")
	}
	padLen := int(data[len(data)-1])
	if padLen <= 0 || padLen > len(data) {
		return nil, errors.New("encrypt: invalid padding")
	}
	return data[:len(data)-padLen], nil
}

func bytesRepeat(b byte, count int) []byte {
	out := make([]byte, count)
	for i := range out {
		out[i] = b
	}
	return out
}

func parsePublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("encrypt: invalid public key pem")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("encrypt: not rsa public key")
	}
	return key, nil
}

func parsePrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, errors.New("encrypt: invalid private key pem")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

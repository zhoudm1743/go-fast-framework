package utils

import (
	"encoding/csv"
	"os"
)

// CsvUtil CSV 工具集。
var CsvUtil = csvUtil{}

type csvUtil struct{}

func (r csvUtil) Read(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer FileUtil.Ignore(f.Close)
	return csv.NewReader(f).ReadAll()
}

func (r csvUtil) Write(path string, records [][]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer FileUtil.Ignore(f.Close)
	w := csv.NewWriter(f)
	if err := w.WriteAll(records); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

func (r csvUtil) ReadMaps(path string) ([]map[string]string, error) {
	rows, err := r.Read(path)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	header := rows[0]
	out := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		m := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(row) {
				m[h] = row[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func (r csvUtil) WriteMaps(path string, header []string, rows []map[string]string) error {
	records := make([][]string, 0, len(rows)+1)
	records = append(records, header)
	for _, row := range rows {
		line := make([]string, len(header))
		for i, h := range header {
			line[i] = row[h]
		}
		records = append(records, line)
	}
	return r.Write(path, records)
}

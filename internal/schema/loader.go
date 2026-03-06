package schema

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"runtime"
)

func manifestPath(name string) string {
	_, currentFile, _, _ := runtime.Caller(0)
	baseDir := filepath.Join(filepath.Dir(currentFile), "..", "..", "schema")
	return filepath.Join(baseDir, name+".json")
}

func Load(name string) (map[string]interface{}, error) {
	filePath := manifestPath(name)
	raw, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", name, err)
	}

	var manifest map[string]interface{}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("decode schema %s: %w", name, err)
	}

	return manifest, nil
}

package buildinfo

import (
	"encoding/json"
	"fmt"
	"io"
)

// Version 由 pnpm run build 从仓库 VERSION 文件替换。
var Version = "development"

// WriteVersion 以文本或稳定 JSON 形式写出某个 CLI 的构建版本。
func WriteVersion(output io.Writer, name string, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(output).Encode(struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{name, Version})
	}
	_, err := fmt.Fprintf(output, "%s %s\n", name, Version)
	return err
}

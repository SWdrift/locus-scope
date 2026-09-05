package buildinfo

import (
	"encoding/json"
	"fmt"
	"io"
)

// Version is replaced from the repository VERSION file by scripts/local-build.ps1.
var Version = "development"

// WriteVersion writes one CLI's build version in text or stable JSON form.
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

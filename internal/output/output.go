package output

import (
	"encoding/json"
	"fmt"
	"os"
)

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

type Envelope struct {
	OK      bool        `json:"ok"`
	Command string      `json:"command"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
}

func IsTTY() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (info.Mode() & os.ModeCharDevice) != 0
}

func ResolveFormat(explicit string, env string) string {
	if explicit != "" {
		return explicit
	}

	if env != "" {
		return env
	}

	if IsTTY() {
		return "human"
	}

	return "json"
}

func PrintJSON(value interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func PrintHuman(format string, message string, data interface{}) error {
	switch format {
	case "json":
		return PrintJSON(data)
	default:
		if message != "" {
			_, err := fmt.Fprintln(os.Stdout, message)
			return err
		}
		return PrintJSON(data)
	}
}

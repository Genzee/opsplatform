package fixture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func strictDecode(data []byte, target any) error {
	if len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		return fmt.Errorf("fixture must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("fixture must contain exactly one JSON object")
	}
	return nil
}

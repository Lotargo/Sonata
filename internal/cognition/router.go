package cognition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

func DecodeRouterOutput(data []byte) (RouterOutput, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	start, err := decoder.Token()
	if err != nil {
		return RouterOutput{}, err
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return RouterOutput{}, errors.New("router output must be a JSON object")
	}

	var output RouterOutput
	seenRoute := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return RouterOutput{}, err
		}
		key, ok := token.(string)
		if !ok {
			return RouterOutput{}, errors.New("router output contains an invalid field name")
		}
		if key != "route" {
			return RouterOutput{}, fmt.Errorf("router output contains unknown field %q", key)
		}
		if seenRoute {
			return RouterOutput{}, errors.New("router output repeats route")
		}
		var route string
		if err := decoder.Decode(&route); err != nil {
			return RouterOutput{}, err
		}
		output.Route = Route(route)
		seenRoute = true
	}

	end, err := decoder.Token()
	if err != nil {
		return RouterOutput{}, err
	}
	if delimiter, ok := end.(json.Delim); !ok || delimiter != '}' {
		return RouterOutput{}, errors.New("router output has an invalid object boundary")
	}
	if !seenRoute {
		return RouterOutput{}, errors.New("router output is missing route")
	}
	if err := ensureRouterJSONEOF(decoder); err != nil {
		return RouterOutput{}, err
	}
	if err := output.Validate(); err != nil {
		return RouterOutput{}, err
	}
	return output, nil
}

func ensureRouterJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("router output must contain one JSON object")
		}
		return err
	}
	return nil
}

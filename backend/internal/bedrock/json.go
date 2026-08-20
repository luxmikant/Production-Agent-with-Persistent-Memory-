package bedrock

import "encoding/json"

func jsonMarshal(v any) ([]byte, error)       { return json.Marshal(v) }
func jsonDecode(b []byte, v any) error       { return json.Unmarshal(b, v) }

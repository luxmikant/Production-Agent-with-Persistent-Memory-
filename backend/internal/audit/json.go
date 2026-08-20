package audit

import "encoding/json"

func jsonM(v any) ([]byte, error) { return json.Marshal(v) }

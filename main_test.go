package main

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

func TestFixStructuredOutputStrictStopHookMismatchLowersStrictOnly(t *testing.T) {
	body := structuredBody(stopHookMismatchedSchema(), nil)
	before := decodeObject(t, body)

	out, changed, err := fixStructuredOutputStrict(body)
	if err != nil {
		t.Fatalf("fixStructuredOutputStrict returned error: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}

	after := decodeObject(t, out)
	format := textFormat(t, after)
	if format["strict"] != false {
		t.Fatalf("strict = %v, want false", format["strict"])
	}
	if format["type"] != "json_schema" {
		t.Fatalf("type = %v, want json_schema", format["type"])
	}
	if format["name"] != "cli_proxy_structured_output" {
		t.Fatalf("name = %v, want cli_proxy_structured_output", format["name"])
	}
	if !reflect.DeepEqual(format["schema"], textFormat(t, before)["schema"]) {
		t.Fatalf("schema changed; want schema semantics preserved")
	}
}

func TestFixStructuredOutputStrictCompatibleNoChange(t *testing.T) {
	body := structuredBody(strictCompatibleSchema(), nil)

	out, changed, err := fixStructuredOutputStrict(body)
	if err != nil {
		t.Fatalf("fixStructuredOutputStrict returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false")
	}
	if len(out) != 0 && !bytes.Equal(out, body) {
		t.Fatalf("unchanged output differs from input")
	}
}

func TestFixStructuredOutputStrictNestedAndDefsMismatchLowerStrict(t *testing.T) {
	cases := []struct {
		name   string
		schema map[string]any
	}{
		{
			name: "nested properties missing from required",
			schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"outer": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"present": map[string]any{"type": "string"},
							"missing": map[string]any{"type": "string"},
						},
						"required": []any{"present"},
					},
				},
				"required": []any{"outer"},
			},
		},
		{
			name: "$defs properties missing from required",
			schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"item": map[string]any{"$ref": "#/$defs/Item"}},
				"required":   []any{"item"},
				"$defs": map[string]any{
					"Item": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":    map[string]any{"type": "string"},
							"label": map[string]any{"type": "string"},
						},
						"required": []any{"id"},
					},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := structuredBody(c.schema, nil)

			out, changed, err := fixStructuredOutputStrict(body)
			if err != nil {
				t.Fatalf("fixStructuredOutputStrict returned error: %v", err)
			}
			if !changed {
				t.Fatalf("changed = false, want true")
			}
			if got := textFormat(t, decodeObject(t, out))["strict"]; got != false {
				t.Fatalf("strict = %v, want false", got)
			}
		})
	}
}

func TestFixStructuredOutputStrictAllowsExtraRequiredNames(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok":         map[string]any{"type": "boolean"},
			"reason":     map[string]any{"type": "string"},
			"impossible": map[string]any{"type": "boolean"},
		},
		"required": []any{"ok", "reason", "impossible", "extra_name"},
	}
	body := structuredBody(schema, nil)

	_, changed, err := fixStructuredOutputStrict(body)
	if err != nil {
		t.Fatalf("fixStructuredOutputStrict returned error: %v", err)
	}
	if changed {
		t.Fatalf("changed = true, want false when every property is required")
	}
}

func TestNormalizeRequestNoopCasesReturnOriginalBody(t *testing.T) {
	mismatch := stopHookMismatchedSchema()
	cases := []struct {
		name string
		from string
		to   string
		body []byte
	}{
		{name: "strict false", from: "claude", to: "codex", body: structuredBodyWithFormat(mismatch, map[string]any{"strict": false})},
		{name: "strict missing", from: "claude", to: "codex", body: structuredBodyWithFormat(mismatch, map[string]any{"omitStrict": true})},
		{name: "strict non bool", from: "claude", to: "codex", body: structuredBodyWithFormat(mismatch, map[string]any{"strict": "true"})},
		{name: "custom name", from: "claude", to: "codex", body: structuredBodyWithFormat(mismatch, map[string]any{"name": "custom_output"})},
		{name: "missing name", from: "claude", to: "codex", body: structuredBodyWithFormat(mismatch, map[string]any{"omitName": true})},
		{name: "non json schema", from: "claude", to: "codex", body: structuredBodyWithFormat(mismatch, map[string]any{"type": "json_object"})},
		{name: "non claude source", from: "openai", to: "codex", body: structuredBody(mismatch, nil)},
		{name: "non codex target", from: "claude", to: "openai", body: structuredBody(mismatch, nil)},
		{name: "ordinary claude request", from: "claude", to: "codex", body: mustJSON(t, map[string]any{"model": "sample-model", "messages": []any{map[string]any{"role": "user", "content": "short request"}}})},
		{name: "tools only", from: "claude", to: "codex", body: mustJSON(t, map[string]any{"tools": sampleTools()})},
		{name: "malformed body", from: "claude", to: "codex", body: []byte(`{"text":{"format":`)},
		{name: "schema missing", from: "claude", to: "codex", body: structuredBodyWithFormat(nil, map[string]any{"omitSchema": true})},
		{name: "schema non object", from: "claude", to: "codex", body: structuredBodyWithFormat(nil, map[string]any{"schema": []any{"not", "object"}})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeBody(t, c.from, c.to, c.body)
			if !bytes.Equal(got, c.body) {
				t.Fatalf("body changed; got %s want %s", got, c.body)
			}
		})
	}
}

func TestMalformedBodyAndInvalidSchemaAreSafeNoops(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{name: "malformed json", body: []byte(`{"text":{"format":`)},
		{name: "missing schema", body: structuredBodyWithFormat(nil, map[string]any{"omitSchema": true})},
		{name: "non object schema", body: structuredBodyWithFormat(nil, map[string]any{"schema": "not-an-object"})},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, changed, err := fixStructuredOutputStrict(c.body)
			if err != nil {
				t.Fatalf("fixStructuredOutputStrict returned error: %v", err)
			}
			if changed {
				t.Fatalf("changed = true, want false")
			}
			if len(out) != 0 && !bytes.Equal(out, c.body) {
				t.Fatalf("safe noop returned different body")
			}
		})
	}
}

func TestNormalizeRequestStructuredWithToolsKeepsToolsUnchanged(t *testing.T) {
	body := structuredBody(stopHookMismatchedSchema(), sampleTools())
	beforeTools := decodeObject(t, body)["tools"]

	got := normalizeBody(t, "claude", "codex", body)
	payload := decodeObject(t, got)

	if gotStrict := textFormat(t, payload)["strict"]; gotStrict != false {
		t.Fatalf("strict = %v, want false", gotStrict)
	}
	if !reflect.DeepEqual(payload["tools"], beforeTools) {
		t.Fatalf("tools changed; got %v want %v", payload["tools"], beforeTools)
	}
}

func TestNormalizeRequestEnvelopeReturnsOKAndResultBody(t *testing.T) {
	body := structuredBody(stopHookMismatchedSchema(), nil)
	req, err := json.Marshal(requestTransformRequest{
		FromFormat: "claude",
		ToFormat:   "codex",
		Model:      "sample-model",
		Body:       body,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	raw, err := normalizeRequest(req)
	if err != nil {
		t.Fatalf("normalizeRequest returned error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("invalid envelope json: %v", err)
	}
	if !env.OK {
		t.Fatalf("env.OK = false, want true")
	}
	if env.Error != nil {
		t.Fatalf("env.Error = %v, want nil", env.Error)
	}
	var result payloadResponse
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	if len(result.Body) == 0 {
		t.Fatalf("result.Body is empty")
	}
	if got := textFormat(t, decodeObject(t, result.Body))["strict"]; got != false {
		t.Fatalf("strict = %v, want false", got)
	}
}

func TestSchemaBearingKeywordsDetectNestedMismatch(t *testing.T) {
	mismatchedObject := map[string]any{
		"type":       "object",
		"properties": map[string]any{"optional": map[string]any{"type": "string"}},
	}
	cases := []struct {
		name   string
		schema map[string]any
	}{
		{
			name: "items schema",
			schema: map[string]any{
				"type":  "array",
				"items": mismatchedObject,
			},
		},
		{
			name: "anyOf schema list",
			schema: map[string]any{
				"anyOf": []any{map[string]any{"type": "string"}, mismatchedObject},
			},
		},
		{
			name: "patternProperties schema map",
			schema: map[string]any{
				"type":              "object",
				"properties":        map[string]any{},
				"required":          []any{},
				"patternProperties": map[string]any{".*": mismatchedObject},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !hasMissingRequiredProperty(c.schema) {
				t.Fatalf("hasMissingRequiredProperty = false, want true")
			}
		})
	}
}

func TestHandleMethodRegistrationAndErrors(t *testing.T) {
	for _, method := range []string{"plugin.register", "plugin.reconfigure"} {
		t.Run(method, func(t *testing.T) {
			raw, err := handleMethod(method, nil)
			if err != nil {
				t.Fatalf("handleMethod returned error: %v", err)
			}
			var env envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("invalid envelope: %v", err)
			}
			if !env.OK || env.Error != nil {
				t.Fatalf("unexpected registration envelope: %+v", env)
			}
			var got registration
			if err := json.Unmarshal(env.Result, &got); err != nil {
				t.Fatalf("invalid registration: %v", err)
			}
			if got.Metadata.Name != pluginID || got.Metadata.Version != pluginVersion {
				t.Fatalf("metadata = %+v", got.Metadata)
			}
			if !got.Capabilities.RequestNormalizer {
				t.Fatalf("request_normalizer capability is false")
			}
		})
	}

	raw, err := handleMethod("unsupported.method", nil)
	if err != nil {
		t.Fatalf("unknown method returned Go error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("invalid error envelope: %v", err)
	}
	if env.OK || env.Error == nil || env.Error.Code != "unknown_method" {
		t.Fatalf("unexpected error envelope: %+v", env)
	}
}

func TestProcessPluginCallReturnsStatusAndEnvelope(t *testing.T) {
	raw, status := processPluginCall("plugin.register", nil)
	if status != 0 {
		t.Fatalf("registration status = %d, want 0", status)
	}
	var success envelope
	if err := json.Unmarshal(raw, &success); err != nil {
		t.Fatalf("invalid success envelope: %v", err)
	}
	if !success.OK {
		t.Fatalf("success.OK = false, want true")
	}

	raw, status = processPluginCall("request.normalize", []byte(`{"Body":`))
	if status != 1 {
		t.Fatalf("malformed request status = %d, want 1", status)
	}
	var failure envelope
	if err := json.Unmarshal(raw, &failure); err != nil {
		t.Fatalf("invalid failure envelope: %v", err)
	}
	if failure.OK || failure.Error == nil || failure.Error.Code != "plugin_error" {
		t.Fatalf("unexpected failure envelope: %+v", failure)
	}
}

func TestNormalizeRequestEnvelopeErrorsAndEmptyBody(t *testing.T) {
	if _, err := normalizeRequest([]byte(`{"Body":`)); err == nil {
		t.Fatalf("malformed envelope error = nil, want error")
	}

	raw, err := normalizeRequest(nil)
	if err != nil {
		t.Fatalf("empty request returned error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("invalid empty-body envelope: %v", err)
	}
	var result payloadResponse
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("invalid empty-body result: %v", err)
	}
	if len(result.Body) != 0 {
		t.Fatalf("empty Body became %q", result.Body)
	}
}

func TestABIErrorPathsAndEnvelopeMarshalError(t *testing.T) {
	main()
	if got := cliproxy_plugin_init(nil, nil); got != 1 {
		t.Fatalf("cliproxy_plugin_init(nil, nil) = %d, want 1", got)
	}
	if got := cliproxyPluginCall(nil, nil, 0, nil); got != 1 {
		t.Fatalf("cliproxyPluginCall with nil method = %d, want 1", got)
	}
	cliproxyPluginFree(nil, 0)
	cliproxyPluginShutdown()
	writeResponse(nil, []byte("ignored"))

	if _, err := okEnvelopeJSON(func() {}); err == nil {
		t.Fatalf("okEnvelopeJSON unsupported value error = nil, want error")
	}
}

func TestFixStructuredOutputStrictPreservesNumberLiterals(t *testing.T) {
	body := []byte(`{"seed":9007199254740993,"text":{"format":{"type":"json_schema","name":"cli_proxy_structured_output","strict":true,"schema":{"type":"object","properties":{"value":{"type":"number","const":9007199254740993,"maximum":0.12345678901234567890123456789}},"required":[],"additionalProperties":false}}},"tools":[{"type":"function","parameters":{"enum":[9007199254740993,0.12345678901234567890123456789]}}]}`)

	out, changed, err := fixStructuredOutputStrict(body)
	if err != nil {
		t.Fatalf("fixStructuredOutputStrict returned error: %v", err)
	}
	if !changed {
		t.Fatalf("changed = false, want true")
	}
	for _, literal := range [][]byte{
		[]byte("9007199254740993"),
		[]byte("0.12345678901234567890123456789"),
	} {
		if !bytes.Contains(out, literal) {
			t.Fatalf("output lost number literal %q: %s", literal, out)
		}
	}
	if bytes.Contains(out, []byte("9007199254740992")) {
		t.Fatalf("output rounded a large integer: %s", out)
	}
}

func TestAdditionalSchemaKeywordsDetectNestedMismatch(t *testing.T) {
	mismatchedObject := map[string]any{
		"type":       "object",
		"properties": map[string]any{"optional": map[string]any{"type": "string"}},
	}
	for _, keyword := range []string{
		"propertyNames",
		"unevaluatedProperties",
		"unevaluatedItems",
		"additionalItems",
		"contentSchema",
		"dependencies",
	} {
		t.Run(keyword, func(t *testing.T) {
			child := any(mismatchedObject)
			if keyword == "dependencies" {
				child = map[string]any{"field": mismatchedObject}
			}
			schema := map[string]any{keyword: child}
			if !hasMissingRequiredProperty(schema) {
				t.Fatalf("hasMissingRequiredProperty(%s) = false, want true", keyword)
			}
		})
	}
}

func TestRequestLengthSupported(t *testing.T) {
	if !requestLengthSupported(0) || !requestLengthSupported(maxCGoBytesLength) {
		t.Fatalf("supported request length was rejected")
	}
	if requestLengthSupported(maxCGoBytesLength + 1) {
		t.Fatalf("oversized request length was accepted")
	}
}

func normalizeBody(t *testing.T, from string, to string, body []byte) []byte {
	t.Helper()
	req, err := json.Marshal(requestTransformRequest{
		FromFormat: from,
		ToFormat:   to,
		Model:      "sample-model",
		Body:       body,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	raw, err := normalizeRequest(req)
	if err != nil {
		t.Fatalf("normalizeRequest returned error: %v", err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("invalid envelope json: %v", err)
	}
	if !env.OK {
		t.Fatalf("env.OK = false, want true")
	}
	var result payloadResponse
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("invalid result json: %v", err)
	}
	return result.Body
}

func structuredBody(schema map[string]any, tools []any) []byte {
	return structuredBodyWithFormat(schema, map[string]any{"tools": tools})
}

func structuredBodyWithFormat(schema map[string]any, options map[string]any) []byte {
	format := map[string]any{
		"type":   "json_schema",
		"name":   "cli_proxy_structured_output",
		"strict": true,
	}
	if schema != nil {
		format["schema"] = schema
	}
	if typ, ok := options["type"]; ok {
		format["type"] = typ
	}
	if name, ok := options["name"]; ok {
		format["name"] = name
	}
	if strict, ok := options["strict"]; ok {
		format["strict"] = strict
	}
	if schemaOverride, ok := options["schema"]; ok {
		format["schema"] = schemaOverride
	}
	if options["omitName"] == true {
		delete(format, "name")
	}
	if options["omitStrict"] == true {
		delete(format, "strict")
	}
	if options["omitSchema"] == true {
		delete(format, "schema")
	}

	payload := map[string]any{"text": map[string]any{"format": format}}
	if tools, ok := options["tools"].([]any); ok && tools != nil {
		payload["tools"] = tools
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return raw
}

func stopHookMismatchedSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok":         map[string]any{"type": "boolean"},
			"reason":     map[string]any{"type": "string"},
			"impossible": map[string]any{"type": "boolean"},
		},
		"required":             []any{"ok", "reason"},
		"additionalProperties": false,
	}
}

func strictCompatibleSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok":         map[string]any{"type": "boolean"},
			"reason":     map[string]any{"type": "string"},
			"impossible": map[string]any{"type": "boolean"},
		},
		"required":             []any{"ok", "reason", "impossible"},
		"additionalProperties": false,
	}
}

func sampleTools() []any {
	return []any{
		map[string]any{
			"type": "function",
			"name": "sample_tool",
			"input_schema": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []any{"path"},
			},
		},
	}
}

func decodeObject(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	return payload
}

func textFormat(t *testing.T, payload map[string]any) map[string]any {
	t.Helper()
	text, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("missing text object: %v", payload)
	}
	format, ok := text["format"].(map[string]any)
	if !ok {
		t.Fatalf("missing text.format object: %v", text)
	}
	return format
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	return raw
}

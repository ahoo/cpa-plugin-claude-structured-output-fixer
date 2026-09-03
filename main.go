package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const cliproxy_host_api* stored_host;

static void store_host_api(const cliproxy_host_api* host) {
	stored_host = host;
}
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unsafe"
)

const abiVersion uint32 = 1

const (
	pluginID      = "claude-structured-output-fixer"
	pluginVersion = "0.1.0"

	defaultStructuredOutputName = "cli_proxy_structured_output"
	maxCGoBytesLength           = uint64(1<<31 - 1)
)

var schemaMapKeywords = [...]string{
	"properties",
	"$defs",
	"definitions",
	"patternProperties",
	"dependentSchemas",
	"dependencies",
}

var schemaValueKeywords = [...]string{
	"items",
	"prefixItems",
	"contains",
	"additionalProperties",
	"propertyNames",
	"unevaluatedProperties",
	"unevaluatedItems",
	"additionalItems",
	"contentSchema",
	"anyOf",
	"oneOf",
	"allOf",
	"not",
	"if",
	"then",
	"else",
}

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type registration struct {
	SchemaVersion uint32       `json:"schema_version"`
	Metadata      metadata     `json:"metadata"`
	Capabilities  capabilities `json:"capabilities"`
}

type metadata struct {
	Name             string `json:"Name"`
	Version          string `json:"Version"`
	Author           string `json:"Author"`
	GitHubRepository string `json:"GitHubRepository"`
	Logo             string `json:"Logo"`
	ConfigFields     []any  `json:"ConfigFields"`
}

type capabilities struct {
	RequestNormalizer bool `json:"request_normalizer"`
}

type requestTransformRequest struct {
	FromFormat string `json:"FromFormat"`
	ToFormat   string `json:"ToFormat"`
	Model      string `json:"Model"`
	Stream     bool   `json:"Stream"`
	Body       []byte `json:"Body"`
}

type payloadResponse struct {
	Body []byte `json:"Body"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	C.store_host_api(host)
	plugin.abi_version = C.uint32_t(abiVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) (status C.int) {
	status = 1
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	defer func() {
		if recover() != nil {
			writeResponse(response, errorEnvelope("plugin_panic", "plugin call failed"))
			status = 1
		}
	}()
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	var payload []byte
	if request != nil && requestLen > 0 {
		if !requestLengthSupported(uint64(requestLen)) {
			writeResponse(response, errorEnvelope("request_too_large", "request exceeds plugin ABI limit"))
			return 1
		}
		payload = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, callStatus := processPluginCall(C.GoString(method), payload)
	writeResponse(response, raw)
	return C.int(callStatus)
}

func requestLengthSupported(length uint64) bool {
	return length <= maxCGoBytesLength
}

func processPluginCall(method string, payload []byte) ([]byte, int) {
	raw, errHandle := handleMethod(method, payload)
	if errHandle != nil {
		return errorEnvelope("plugin_error", errHandle.Error()), 1
	}
	return raw, 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, len C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
	_ = len
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, payload []byte) ([]byte, error) {
	switch method {
	case "plugin.register", "plugin.reconfigure":
		return okEnvelopeJSON(registration{
			SchemaVersion: abiVersion,
			Metadata: metadata{
				Name:             pluginID,
				Version:          pluginVersion,
				Author:           "cpa-admin",
				GitHubRepository: "https://github.com/ahoo/cpa-plugin-claude-structured-output-fixer",
				Logo:             "",
				ConfigFields:     []any{},
			},
			Capabilities: capabilities{RequestNormalizer: true},
		})
	case "request.normalize":
		return normalizeRequest(payload)
	default:
		return errorEnvelope("unknown_method", "unknown method"), nil
	}
}

func normalizeRequest(payload []byte) ([]byte, error) {
	var req requestTransformRequest
	if len(payload) > 0 {
		if errDecode := json.Unmarshal(payload, &req); errDecode != nil {
			return nil, errDecode
		}
	}
	if len(req.Body) == 0 ||
		!strings.EqualFold(strings.TrimSpace(req.FromFormat), "claude") ||
		!strings.EqualFold(strings.TrimSpace(req.ToFormat), "codex") {
		return okEnvelopeJSON(payloadResponse{Body: req.Body})
	}

	fixed, changed, errFix := fixStructuredOutputStrict(req.Body)
	if errFix != nil {
		return nil, errFix
	}
	if !changed {
		return okEnvelopeJSON(payloadResponse{Body: req.Body})
	}
	return okEnvelopeJSON(payloadResponse{Body: fixed})
}

func fixStructuredOutputStrict(body []byte) ([]byte, bool, error) {
	var payload map[string]any
	if errDecode := decodeJSONUseNumber(body, &payload); errDecode != nil {
		return nil, false, nil
	}

	text, ok := payload["text"].(map[string]any)
	if !ok {
		return nil, false, nil
	}
	format, ok := text["format"].(map[string]any)
	if !ok || format["type"] != "json_schema" || format["name"] != defaultStructuredOutputName {
		return nil, false, nil
	}
	strict, ok := format["strict"].(bool)
	if !ok || !strict {
		return nil, false, nil
	}
	schema, ok := format["schema"].(map[string]any)
	if !ok || !hasMissingRequiredProperty(schema) {
		return nil, false, nil
	}

	format["strict"] = false
	fixed, errMarshal := json.Marshal(payload)
	if errMarshal != nil {
		return nil, false, errMarshal
	}
	return fixed, true, nil
}

func decodeJSONUseNumber(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func hasMissingRequiredProperty(schema any) bool {
	node, ok := schema.(map[string]any)
	if !ok {
		if list, isList := schema.([]any); isList {
			for _, child := range list {
				if hasMissingRequiredProperty(child) {
					return true
				}
			}
		}
		return false
	}

	if properties, hasProperties := node["properties"].(map[string]any); hasProperties {
		if len(properties) > 0 && requiredMissesProperty(node["required"], properties) {
			return true
		}
	}

	for _, keyword := range schemaMapKeywords {
		children, exists := node[keyword].(map[string]any)
		if !exists {
			continue
		}
		for _, child := range children {
			if hasMissingRequiredProperty(child) {
				return true
			}
		}
	}

	for _, keyword := range schemaValueKeywords {
		child, exists := node[keyword]
		if exists && hasMissingRequiredProperty(child) {
			return true
		}
	}
	return false
}

func requiredMissesProperty(raw any, properties map[string]any) bool {
	required, ok := raw.([]any)
	if !ok {
		return true
	}
	names := make(map[string]struct{}, len(required))
	for _, item := range required {
		name, isString := item.(string)
		if isString {
			names[name] = struct{}{}
		}
	}
	for name := range properties {
		if _, exists := names[name]; !exists {
			return true
		}
	}
	return false
}

func okEnvelopeJSON(result any) ([]byte, error) {
	raw, errMarshal := json.Marshal(result)
	if errMarshal != nil {
		return nil, errMarshal
	}
	return json.Marshal(envelope{OK: true, Result: json.RawMessage(raw)})
}

func errorEnvelope(code, message string) []byte {
	raw, _ := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

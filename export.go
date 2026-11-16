// Derived from jellyfish-p/cpa-plugin-antigravity-coding-filter (MIT).
// The C ABI below is the CLIProxyAPI v7 dynamic-plugin contract; the
// identity rewrite itself lives in the other files of this package.
package main

/*
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, char*, uint8_t*, size_t, cliproxy_buffer*);
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

extern int cliproxy_plugin_call(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxy_plugin_free(void*, size_t);
extern void cliproxy_plugin_shutdown(void);

static int cliproxy_call_host(cliproxy_host_api* api, char* method, uint8_t* request, size_t request_len, cliproxy_buffer* response) {
	if (api == 0 || api->call == 0 || method == 0) {
		return 1;
	}
	return api->call(api->host_ctx, method, request, request_len, response);
}

static void cliproxy_free_host_buffer(cliproxy_host_api* api, void* ptr, size_t len) {
	if (api == 0 || api->free_buffer == 0 || ptr == 0) {
		return;
	}
	api->free_buffer(ptr, len);
}
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
)

const abiVersion = 1

var (
	hostSlotMu sync.Mutex
	hostSlot   *C.cliproxy_host_api
)

func main() {}

func storeHostAPI(api *C.cliproxy_host_api) {
	hostSlotMu.Lock()
	hostSlot = api
	hostSlotMu.Unlock()
}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	storeHostAPI(host)
	plugin.abi_version = abiVersion
	plugin.call = (C.cliproxy_plugin_call_fn)(C.cliproxy_plugin_call)
	plugin.free_buffer = (C.cliproxy_plugin_free_fn)(C.cliproxy_plugin_free)
	plugin.shutdown = (C.cliproxy_plugin_shutdown_fn)(C.cliproxy_plugin_shutdown)
	return 0
}

//export cliproxy_plugin_call
func cliproxy_plugin_call(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeCResponse(response, mustErrorEnvelope("invalid_method", "method is required"))
		return 1
	}

	methodName := C.GoString(method)
	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}

	raw, code := handlePluginCall(methodName, requestBytes)
	writeCResponse(response, raw)
	return C.int(code)
}

//export cliproxy_plugin_free
func cliproxy_plugin_free(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxy_plugin_shutdown
func cliproxy_plugin_shutdown() {
	storeHostAPI(nil)
}

// sendHostLog emits one privacy-filtered diagnostic through the existing
// host.log callback. It is a no-op until the host passes its API at init,
// and it never sends the request body.
func sendHostLog(rec diagRecord) {
	payload, err := diagnosticPayload(rec)
	if err != nil || len(payload) == 0 {
		return
	}
	hostSlotMu.Lock()
	defer hostSlotMu.Unlock()
	api := hostSlot
	if api == nil || api.call == nil {
		return
	}
	method := C.CString(pluginabi.MethodHostLog)
	defer C.free(unsafe.Pointer(method))
	request := C.CBytes(payload)
	defer C.free(request)
	var response C.cliproxy_buffer
	_ = C.cliproxy_call_host(api, method, (*C.uint8_t)(request), C.size_t(len(payload)), &response)
	if response.ptr != nil {
		C.cliproxy_free_host_buffer(api, response.ptr, response.len)
	}
}

func writeCResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.malloc(C.size_t(len(raw)))
	if ptr == nil {
		return
	}
	C.memcpy(ptr, unsafe.Pointer(&raw[0]), C.size_t(len(raw)))
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}

//go:build darwin

package coreml

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -I${SRCDIR}
#cgo LDFLAGS: -framework Foundation -framework CoreML
#include "bridge.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"

import (
	"fmt"
	"unsafe"
)

type Model struct {
	ptr *C.laya_ml_model
}

const (
	UnitsAll    = 0
	UnitsCPU    = 1
	UnitsCPUGPU = 2
	UnitsCPUNE  = 3
)

func Load(path string, computeUnits int) (*Model, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	var errMsg *C.char
	ptr := C.laya_ml_load(cPath, C.int(computeUnits), &errMsg)
	if ptr == nil {
		msg := "failed to load Core ML model"
		if errMsg != nil {
			msg = C.GoString(errMsg)
			C.laya_ml_free_string(errMsg)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return &Model{ptr: ptr}, nil
}

func (m *Model) Close() {
	if m == nil || m.ptr == nil {
		return
	}
	C.laya_ml_free(m.ptr)
	m.ptr = nil
}

type Input struct {
	Name  string
	Shape []int64
	Data  []byte // float16 LE
}

type Output struct {
	Name  string
	Shape []int64
	Data  []float32
}

func (m *Model) Predict(inputs []Input) ([]Output, error) {
	if m == nil || m.ptr == nil {
		return nil, fmt.Errorf("model is closed")
	}
	cInputs := make([]C.laya_ml_input, len(inputs))
	type alloc struct {
		name  *C.char
		shape unsafe.Pointer
		data  unsafe.Pointer
	}
	allocs := make([]alloc, len(inputs))
	defer func() {
		for _, a := range allocs {
			if a.name != nil {
				C.free(unsafe.Pointer(a.name))
			}
			if a.shape != nil {
				C.free(a.shape)
			}
			if a.data != nil {
				C.free(a.data)
			}
		}
	}()

	for i, in := range inputs {
		name := C.CString(in.Name)
		shapeBytes := C.size_t(len(in.Shape) * int(unsafe.Sizeof(C.int64_t(0))))
		shapePtr := C.malloc(shapeBytes)
		if shapePtr == nil {
			return nil, fmt.Errorf("oom shape")
		}
		if len(in.Shape) > 0 {
			dst := unsafe.Slice((*C.int64_t)(shapePtr), len(in.Shape))
			for j, v := range in.Shape {
				dst[j] = C.int64_t(v)
			}
		}
		dataPtr := C.malloc(C.size_t(len(in.Data)))
		if dataPtr == nil {
			return nil, fmt.Errorf("oom data")
		}
		if len(in.Data) > 0 {
			C.memcpy(dataPtr, unsafe.Pointer(&in.Data[0]), C.size_t(len(in.Data)))
		}
		allocs[i] = alloc{name: name, shape: shapePtr, data: dataPtr}
		cInputs[i] = C.laya_ml_input{
			name:   name,
			shape:  (*C.int64_t)(shapePtr),
			ndim:   C.int(len(in.Shape)),
			data:   dataPtr,
			nbytes: C.size_t(len(in.Data)),
		}
	}

	var outs *C.laya_ml_output
	var nOut C.int
	var errMsg *C.char
	rc := C.laya_ml_predict(m.ptr, &cInputs[0], C.int(len(cInputs)), &outs, &nOut, &errMsg)
	if rc != 0 {
		msg := "Core ML prediction failed"
		if errMsg != nil {
			msg = C.GoString(errMsg)
			C.laya_ml_free_string(errMsg)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	defer C.laya_ml_free_outputs(outs, nOut)

	result := make([]Output, int(nOut))
	slice := unsafe.Slice(outs, int(nOut))
	for i := 0; i < int(nOut); i++ {
		o := &slice[i]
		shape := make([]int64, int(o.ndim))
		oshape := unsafe.Slice(o.shape, int(o.ndim))
		for d := 0; d < int(o.ndim); d++ {
			shape[d] = int64(oshape[d])
		}
		data := make([]float32, int(o.nelem))
		copy(data, unsafe.Slice((*float32)(unsafe.Pointer(o.data)), int(o.nelem)))
		result[i] = Output{
			Name:  C.GoString(o.name),
			Shape: shape,
			Data:  data,
		}
	}
	return result, nil
}

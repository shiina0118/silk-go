package silk

import (
	"cmp"
	"reflect"
	"unsafe"
)

var magicV3 = []byte{0x02, 0x23, 0x21, 0x53, 0x49, 0x4C, 0x4B, 0x5F, 0x56, 0x33}

type slice[T any] struct {
	arr          []T
	offset, size int
}

func (s *slice[T]) From() []T {
	return s.arr[s.offset:]
}

func (s *slice[T]) ptr(offset int) *T {
	addr := s._addr(offset)
	if addr < 0 {
		return (*T)(nil)
	}
	return &s.arr[addr]
}

func (s *slice[T]) idx(offset int) T {
	// Indexing already checks both bounds; avoid the extra pointer indirection
	// and duplicate checks on every sample in the fixed-point filters.
	return s.arr[s.offset+offset]
}

func (s *slice[T]) off(offset int) *slice[T] {
	addr := s._addr(offset)
	if addr < 0 {
		return nil
	}
	return &slice[T]{
		arr:    s.arr,
		offset: addr,
		size:   s.size,
	}
}

// offInto is off without allocating: the view is written to dst, normally a
// caller-owned local reused across loop iterations, and dst is returned.
// Out-of-range offsets return nil exactly as off does. s may equal dst.
func (s *slice[T]) offInto(dst *slice[T], offset int) *slice[T] {
	addr := s._addr(offset)
	if addr < 0 {
		return nil
	}
	*dst = slice[T]{
		arr:    s.arr,
		offset: addr,
		size:   s.size,
	}
	return dst
}

func (s *slice[T]) copy(dest *slice[T], size int) {
	destSlice := dest.From()
	sSlice := s.From()
	copy(destSlice[:size], sSlice[:size])
}

func (s *slice[T]) _addr(offset int) int {
	off := s.offset + offset
	if off < 0 || off > len(s.arr) {
		return -1
	}
	return off
}

func slice2[U, T any](s *slice[T]) *slice[U] {
	var element U
	size := int(unsafe.Sizeof(element))

	off := s.arr[s.offset:]

	sh := (*reflect.SliceHeader)(unsafe.Pointer(&off))
	slen, scap := (sh.Len*s.size)/size, (sh.Cap*s.size)/size
	sh.Len = slen
	sh.Cap = scap

	return &slice[U]{
		arr:  *(*[]U)(unsafe.Pointer(sh)),
		size: size,
	}
}

func alloc[T any](size int) *slice[T] {
	var element T
	eSize := int(unsafe.Sizeof(element))
	arr := make([]T, size, size)
	return &slice[T]{
		arr:  arr,
		size: eSize,
	}
}

func memcpy[T any](dst, src []T, size int) {
	copy(dst[:size], src[:size])
}

func mem2Slice[T any](a []T) *slice[T] {
	var element T
	return &slice[T]{arr: a, size: int(unsafe.Sizeof(element))}
}

func matrix_ptr[T any](s *slice[T], row, column, N int32) *T {
	return s.ptr(int(row*N + column))
}

func matrix_adr[T any](s *slice[T], row, column, N int32) *slice[T] {
	return s.off(int(row*N + column))
}

func memset[T any](s *slice[T], v T, size int) {
	if size <= 0 {
		return
	}
	data := s.arr[s.offset : s.offset+size]
	for i := range data {
		data[i] = v
	}
}

func min[T cmp.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

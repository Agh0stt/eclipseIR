package main

// ─────────────────────────────────────────────
//  types.go — EclipseIR Type System
// ─────────────────────────────────────────────

// Type represents a primitive EclipseIR value type.
type Type int

const (
	TY_VOID Type = iota
	TY_I8
	TY_I16
	TY_I32
	TY_I64
	TY_U8
	TY_U16
	TY_U32
	TY_U64
	TY_F32
	TY_F64
	TY_BOOL
	TY_STR
	TY_PTR  // pointer (void*)
)

func (t Type) String() string {
	switch t {
	case TY_VOID:
		return "void"
	case TY_I8:
		return "i8"
	case TY_I16:
		return "i16"
	case TY_I32:
		return "i32"
	case TY_I64:
		return "i64"
	case TY_U8:
		return "u8"
	case TY_U16:
		return "u16"
	case TY_U32:
		return "u32"
	case TY_U64:
		return "u64"
	case TY_F32:
		return "f32"
	case TY_F64:
		return "f64"
	case TY_BOOL:
		return "bool"
	case TY_STR:
		return "str"
	case TY_PTR:
		return "ptr"
	default:
		return "unknown"
	}
}

// IsFloat returns true for f32/f64.
func (t Type) IsFloat() bool { return t == TY_F32 || t == TY_F64 }

// IsInt returns true for all integer widths.
func (t Type) IsInt() bool {
	switch t {
	case TY_I8, TY_I16, TY_I32, TY_I64,
		TY_U8, TY_U16, TY_U32, TY_U64, TY_BOOL:
		return true
	}
	return false
}

// Is64Bit returns true for types that require 64-bit registers.
func (t Type) Is64Bit() bool {
	return t == TY_I64 || t == TY_U64 || t == TY_PTR || t == TY_STR
}

// BitSize returns the bit width of the type.
func (t Type) BitSize() int {
	switch t {
	case TY_I8, TY_U8:
		return 8
	case TY_I16, TY_U16:
		return 16
	case TY_I32, TY_U32, TY_F32, TY_BOOL:
		return 32
	case TY_I64, TY_U64, TY_F64, TY_PTR, TY_STR:
		return 64
	}
	return 0
}

// parseType maps a type string to a Type constant.
func parseType(s string) (Type, bool) {
	switch s {
	case "void":
		return TY_VOID, true
	case "i8":
		return TY_I8, true
	case "i16":
		return TY_I16, true
	case "i32":
		return TY_I32, true
	case "i64":
		return TY_I64, true
	case "u8":
		return TY_U8, true
	case "u16":
		return TY_U16, true
	case "u32":
		return TY_U32, true
	case "u64":
		return TY_U64, true
	case "f32":
		return TY_F32, true
	case "f64":
		return TY_F64, true
	case "bool":
		return TY_BOOL, true
	case "str":
		return TY_STR, true
	case "ptr":
		return TY_PTR, true
	}
	return TY_VOID, false
}

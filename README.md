# EclipseIR v1.1.0

**A custom intermediate representation compiler targeting AArch64 Linux — no libc, raw syscalls only.**

---

## Overview

EclipseIR is a text-based IR that compiles directly to AArch64 GNU assembly, assembled and linked with `as` + `ld` into standalone ELF binaries. No libc dependency. All I/O is done through raw Linux syscalls.

### Pipeline

```
input.ir → Lexer → Parser → AST → Checker → [Optimizer] → Codegen → .s → as → .o → ld → binary
```

### Components

| File | Role |
|------|------|
| `lexer.go` | Token-based lexer for the IR text format |
| `parser.go` | Recursive-descent parser → Program AST |
| `ast.go` | IR node types (Instr, Function, Global, Program) |
| `types.go` | Full type system: i8/i16/i32/i64, u8-u64, f32/f64, bool, str, ptr |
| `checker.go` | Semantic checker: vreg def-use, type consistency, label resolution, call arity |
| `optimizer.go` | Const folding + dead code elimination |
| `regalloc.go` | Virtual→physical register allocator (per type class) |
| `codegen.go` | AArch64 Linux ELF codegen + runtime helpers |
| `logger.go` | CLI logger with ANSI color |
| `main.go` | CLI entry point |

---

## IR Format

### Globals

```
@msg = c"Hello, world!\n"
@count = i32 42
@ratio = f64 3.14
```

### Functions

```
func @add(i32 %0, i32 %1) -> i32 {
    %2 = add i32 %0 %1
    ret i32 %2
}
```

### Instructions

| Instruction | Syntax | Notes |
|-------------|--------|-------|
| `const` | `%0 = const i32 42` | Load immediate |
| `add/sub/mul/div/mod` | `%2 = add i32 %0 %1` | Arithmetic |
| `and/or/xor/not/neg` | `%2 = and i32 %0 %1` | Bitwise/logical |
| `shl/shr` | `%2 = shl i32 %0 %1` | Shifts |
| `gt/lt/eq/ge/le/ne` | `%2 = gt i32 %0 %1` | Comparison → bool |
| `goto` | `goto Llabel` | Unconditional branch |
| `if_goto` | `if_goto %0 Llabel` | Conditional branch |
| `ret` | `ret i32 %0` / `ret` | Return |
| `call` | `%0 = call @fn(%1, %2)` | Function call |
| `syscall` | `%0 = syscall 64 %fd %buf %len` | Raw Linux syscall |
| `puts` | `puts @msg` | Print null-terminated global string |
| `alloc` | `%0 = alloc i32 4` | Stack allocation |
| `load` | `%0 = load i32 %ptr` | Load from pointer |
| `store` | `store i32 %ptr %val` | Store to pointer |
| `label` | `Lname:` | Block label |

### Types

`i8` `i16` `i32` `i64` `u8` `u16` `u32` `u64` `f32` `f64` `bool` `str` `ptr` `void`

### Comments

```
// line comment
; also a line comment
```

---

## CLI

```
eclipseir <input.ir> [flags]

  --emit-asm              Write assembly to .s file
  --emit-ir               Dump parsed IR to stdout
  --out <name>            Build binary (triggers as + ld)
  --asm-file <file>       Override .s output filename
  --output-dir <dir>      Write all outputs to directory
  --optimize              Const fold + dead code elimination
  --no-check              Skip semantic checker
  --no-cleanup            Keep .o file after linking
  --verbose               Show each step
  --stats                 Print IR statistics
  --dry-run               Parse + check only, no codegen
  --no-color              Disable ANSI colors
  --version               Print version
  --help                  Show help
```

---

## Build

```bash
# For AArch64 Linux (cross-compile from x86):
GOOS=linux GOARCH=arm64 go build -o eclipseir .

# For host (dev/testing):
go build -o eclipseir .

# Run tests (requires host build):
make test
```

---

## Syscall ABI (AArch64 Linux)

| Register | Role |
|----------|------|
| `x8` | Syscall number |
| `x0-x5` | Arguments |
| `x0` | Return value |
| `svc #0` | Invoke syscall |

| Number | Name |
|--------|------|
| 63 | `sys_read` |
| 64 | `sys_write` |
| 93 | `sys_exit` |

---

## Runtime Helpers (emitted into every binary)

| Symbol | Description |
|--------|-------------|
| `__eclipse_puts` | `sys_write(1, str, strlen(str))` |
| `__eclipse_write` | Raw `sys_write` wrapper |
| `__eclipse_read` | Raw `sys_read` wrapper |
| `__eclipse_exit` | `sys_exit` wrapper |
| `__eclipse_print_int` | Print i64 as decimal to stdout |

---

## Example

**hello.ir**
```
@msg = c"Hello, EclipseIR!\n"

func @main() -> i32 {
    puts @msg
    %0 = const i32 0
    ret i32 %0
}
```

```bash
# Dump IR + stats
./eclipseir hello.ir --emit-ir --stats

# Emit assembly
./eclipseir hello.ir --emit-asm

# Build binary (on AArch64 Linux)
./eclipseir hello.ir --out hello --verbose

# Build with optimization
./eclipseir hello.ir --out hello --optimize --verbose
```

## What's new in v1.1.0

- **Linear scan register allocator** — replaces the broken `vreg % 20` hack
- **Stack spilling** — when physical registers run out, vregs are spilled to `[x29, #-N]` and reloaded automatically
- **Proper frame sizing** — prologue allocates exact frame based on spill count
- **`strlen` builtin** — `%dst = strlen @global` returns i64 string length
- **`__eclipse_strlen` runtime** — now a real callable helper, used by `__eclipse_puts` too
- **`--emit-liveness`** — dump live ranges per function
- **`--emit-regalloc`** — dump vreg → physical reg / spill slot mapping
- **`xor`, `shl`, `shr`, `neg`** — fully tested codegen paths
- **Call emission** — now resolves callee param types for correct ABI register selection
- **Scratch registers** — moved to w20/x20/s16/d16 to avoid colliding with allocatable pool

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const version = "1.1.0"

type Config struct {
	InputFile     string
	AsmFile       string
	BinFile       string
	EmitAsm       bool
	EmitIR        bool
	EmitHex       bool
	EmitLiveness  bool
	EmitRegAlloc  bool
	Verbose       bool
	NoCleanup     bool
	OutputDir     string
	Optimize      bool
	DryRun        bool
	StatsDump     bool
	ColorOff      bool
	NoCheck       bool
}

func usage() {
	fmt.Print(`EclipseIR Compiler — AArch64 Linux (no libc)
Version: ` + version + `

Usage:
  eclipseir <input.ir> [flags]

Flags:
  --emit-asm              Write generated assembly to a .s file
  --emit-ir               Dump parsed IR to stdout before codegen
  --emit-liveness         Dump live ranges per function
  --emit-regalloc         Dump vreg → physical reg / spill slot mapping
  --out <n>               Output binary name (triggers full build via as + ld)
  --asm-file <file>       Override default assembly filename
  --output-dir <dir>      Place all outputs in this directory
  --optimize              Apply constant folding + dead code elimination
  --no-check              Skip semantic type checker
  --no-cleanup            Keep intermediate .o file after binary build
  --verbose               Print each compilation step in detail
  --stats                 Print IR statistics
  --dry-run               Parse and check only — no codegen
  --no-color              Disable ANSI color output
  --version               Print version and exit
  --help                  Show this help message

Syscalls used (no libc):
  sys_read #63   sys_write #64   sys_exit #93

Runtime helpers emitted into every binary:
  __eclipse_puts        print null-terminated string to stdout
  __eclipse_strlen      strlen (returns i64)
  __eclipse_write       raw sys_write wrapper
  __eclipse_read        raw sys_read wrapper
  __eclipse_exit        sys_exit wrapper
  __eclipse_print_int   print i64 as decimal

IR instructions:
  const  add  sub  mul  div  mod  neg
  and  or  xor  not  shl  shr
  gt  lt  eq  ge  le  ne
  goto  if_goto  ret  call  syscall
  puts  strlen  alloc  load  store

Examples:
  eclipseir prog.ir --emit-ir --stats
  eclipseir prog.ir --emit-asm
  eclipseir prog.ir --emit-liveness --emit-regalloc --verbose
  eclipseir prog.ir --out prog --verbose
  eclipseir prog.ir --out prog --optimize --verbose
  eclipseir prog.ir --dry-run
`)
}

func parseArgs(args []string) (*Config, error) {
	if len(args) == 0 {
		usage()
		os.Exit(0)
	}
	cfg := &Config{}
	i := 0
	for i < len(args) {
		arg := args[i]
		switch arg {
		case "--help", "-h":
			usage()
			os.Exit(0)
		case "--version":
			fmt.Println("EclipseIR version " + version)
			os.Exit(0)
		case "--emit-asm":
			cfg.EmitAsm = true
		case "--emit-ir":
			cfg.EmitIR = true
		case "--emit-hex":
			cfg.EmitHex = true
		case "--emit-liveness":
			cfg.EmitLiveness = true
		case "--emit-regalloc":
			cfg.EmitRegAlloc = true
		case "--verbose":
			cfg.Verbose = true
		case "--no-cleanup":
			cfg.NoCleanup = true
		case "--dry-run":
			cfg.DryRun = true
		case "--stats":
			cfg.StatsDump = true
		case "--no-color":
			cfg.ColorOff = true
		case "--optimize":
			cfg.Optimize = true
		case "--no-check":
			cfg.NoCheck = true
		case "--out":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--out requires an argument")
			}
			cfg.BinFile = args[i]
		case "--asm-file":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--asm-file requires an argument")
			}
			cfg.AsmFile = args[i]
		case "--output-dir":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--output-dir requires an argument")
			}
			cfg.OutputDir = args[i]
		default:
			if strings.HasPrefix(arg, "--") {
				return nil, fmt.Errorf("unknown flag: %s (try --help)", arg)
			}
			if cfg.InputFile != "" {
				return nil, fmt.Errorf("unexpected argument: %s", arg)
			}
			cfg.InputFile = arg
		}
		i++
	}
	if cfg.InputFile == "" {
		return nil, fmt.Errorf("no input file specified")
	}
	if cfg.AsmFile == "" {
		base := strings.TrimSuffix(filepath.Base(cfg.InputFile), filepath.Ext(cfg.InputFile))
		cfg.AsmFile = base + ".s"
	}
	if cfg.OutputDir != "" {
		if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("cannot create output dir: %w", err)
		}
		cfg.AsmFile = filepath.Join(cfg.OutputDir, filepath.Base(cfg.AsmFile))
		if cfg.BinFile != "" {
			cfg.BinFile = filepath.Join(cfg.OutputDir, filepath.Base(cfg.BinFile))
		}
	}
	return cfg, nil
}

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31m[error]\033[0m %s\n", err)
		os.Exit(1)
	}

	log := NewLogger(cfg)

	// 1. Parse
	log.Step("Parse", cfg.InputFile)
	program, err := ParseIR(cfg.InputFile)
	if err != nil {
		log.Error("Parse failed:\n" + err.Error())
		os.Exit(1)
	}
	log.Ok(fmt.Sprintf("Parsed — %d function(s), %d global(s)",
		len(program.Funcs), len(program.Globals)))

	// 2. Stats
	if cfg.StatsDump {
		program.PrintStats(log)
	}

	// 3. IR dump
	if cfg.EmitIR {
		fmt.Println()
		program.DumpIR()
		fmt.Println()
	}

	// 4. Semantic check
	if !cfg.NoCheck {
		log.Step("Check", "semantic analysis")
		checker := NewChecker(program)
		errs := checker.Check()
		if len(errs) > 0 {
			log.Error(fmt.Sprintf("%d check error(s):", len(errs)))
			fmt.Fprintln(os.Stderr, FormatCheckErrors(errs))
			os.Exit(1)
		}
		log.Ok("Semantic check passed")
	}

	// 5. Dry run
	if cfg.DryRun {
		log.Info("Dry run — stopping before codegen.")
		return
	}

	// 6. Optimize
	if cfg.Optimize {
		log.Step("Optimize", "const fold + dead code elim")
		program.Optimize()
		log.Ok("Optimization pass complete")
	}

	// 7. Liveness + regalloc dumps (run alloc but don't emit asm)
	if cfg.EmitLiveness || cfg.EmitRegAlloc {
		ra := NewRegAlloc()
		for i := range program.Funcs {
			fn := &program.Funcs[i]
			ra.InitForFunc(fn, func(f string, a ...interface{}) {}) // silent emitter
			fmt.Printf("── %s ──\n", fn.Name)
			if cfg.EmitLiveness {
				fmt.Printf("  Live ranges:\n%s", ra.DumpLiveness())
			}
			if cfg.EmitRegAlloc {
				fmt.Printf("  Register allocation:\n%s", ra.DumpAlloc())
			}
		}
		fmt.Println()
	}

	// 8. Codegen
	if cfg.EmitAsm || cfg.BinFile != "" {
		log.Step("Codegen", cfg.AsmFile)
		cg := NewCodegen(program)
		asm := cg.Emit()
		if err := os.WriteFile(cfg.AsmFile, []byte(asm), 0644); err != nil {
			log.Error("Failed to write assembly: " + err.Error())
			os.Exit(1)
		}
		log.Ok("Assembly → " + cfg.AsmFile)
	}

	// 9. Assemble + link
	if cfg.BinFile != "" {
		log.Step("Assemble", cfg.AsmFile+" → "+cfg.BinFile)
		if err := AssembleAndLink(cfg, log); err != nil {
			log.Error(err.Error())
			os.Exit(1)
		}
		log.Ok("Binary → ./" + cfg.BinFile)
	}

	if !cfg.EmitAsm && !cfg.EmitIR && cfg.BinFile == "" &&
		!cfg.StatsDump && !cfg.DryRun &&
		!cfg.EmitLiveness && !cfg.EmitRegAlloc {
		log.Warn("No output requested. Try --emit-asm, --emit-ir, --out <bin>, --stats, or --emit-regalloc.")
	}
}

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h> 

typedef enum { TY_I32, TY_F32, TY_F64, TY_BOOL, TY_STR, TY_VOID } Type;

typedef enum {
    INS_CONST, INS_ADD, INS_SUB, INS_MUL, INS_DIV,
    INS_CMP_GT, INS_CMP_LT, INS_CMP_EQ, INS_CMP_GE, INS_CMP_LE, INS_CMP_NE,
    INS_PUTS, INS_RET, INS_LABEL, INS_GOTO, INS_IF_GOTO, INS_CALL, INS_MOD
} InstrKind;

typedef struct {
    InstrKind kind;
    int dst;
    int *src_args;
    int arg_count;
    int imm;
    Type type;
    char label[128];
} Instr;

typedef struct {
    char name[128];
    Type ret_type;
    Instr *instrs;
    int instr_count;
    int instr_cap;
} Function;

typedef struct {
    char name[128];
    char value[256];
    Type type;
} Global;

// Dynamic Arrays
Global *globals = NULL;
int global_count = 0;
int global_cap = 0;

Function *funcs = NULL;
int func_count = 0;
int func_cap = 0;

/* --- Memory Management --- */
void add_global(Global g) {
    if (global_count >= global_cap) {
        global_cap = (global_cap == 0) ? 16 : global_cap * 2;
        globals = realloc(globals, sizeof(Global) * global_cap);
    }
    globals[global_count++] = g;
}

int add_func(char *name, Type ret) {
    if (func_count >= func_cap) {
        func_cap = (func_cap == 0) ? 8 : func_cap * 2;
        funcs = realloc(funcs, sizeof(Function) * func_cap);
    }
    memset(&funcs[func_count], 0, sizeof(Function));
    strcpy(funcs[func_count].name, name);
    funcs[func_count].ret_type = ret;
    return func_count++;
}

void add_instr(Function *f, Instr in) {
    if (f->instr_count >= f->instr_cap) {
        f->instr_cap = (f->instr_cap == 0) ? 32 : f->instr_cap * 2;
        f->instrs = realloc(f->instrs, sizeof(Instr) * f->instr_cap);
    }
    f->instrs[f->instr_count++] = in;
}

/* --- Register Management --- */
// Simple modulo allocator (Warning: Potential collisions if vreg > 30)
const char* rname(int v, Type t) {
    static char buf[16][32];
    static int bidx = 0;
    bidx = (bidx + 1) % 16;
    int p = (v < 0) ? 0 : (v % 31); 
    
    if (t == TY_F32) sprintf(buf[bidx], "s%d", p);      // Single precision
    else if (t == TY_F64) sprintf(buf[bidx], "d%d", p); // Double precision
    else if (t == TY_I32 || t == TY_BOOL) sprintf(buf[bidx], "w%d", p); // 32-bit int
    else sprintf(buf[bidx], "x%d", p); // 64-bit int/ptr
    return buf[bidx];
}

void parse_ir(const char *file) {
    FILE *f = fopen(file, "r");
    if (!f) { perror("Failed to open file"); return; }
    char line[512];
    int cur = -1;

    while (fgets(line, sizeof(line), f)) {
        if (line[0] == '@') {
            Global g;
            if (strstr(line, "c\"")) {
                sscanf(line, "@%127s = c\"%255[^\"]\"", g.name, g.value);
                g.type = TY_STR;
            } else {
                sscanf(line, "@%127s = i32 %255s", g.name, g.value);
                g.type = TY_I32;
            }
            add_global(g);
            continue;
        }

        if (strstr(line, "func")) {
            char name[128], type_str[16];
            sscanf(line, "func @%127[^( ](%*[^)]) -> %15s", name, type_str);
            cur = add_func(name, (strcmp(type_str, "i32") == 0) ? TY_I32 : TY_VOID);
            continue;
        }

        if (cur == -1 || line[0] == '}' || line[0] == '\n') continue;

        Instr in = {0};
        in.dst = -1;
        in.src_args = malloc(sizeof(int) * 8);
        for(int i=0; i<8; i++) in.src_args[i] = -1;

        if (strstr(line, "const")) {
            in.kind = INS_CONST;
            char ty[16];
            double val; 
            sscanf(line, " %%%d = const %15s %lf", &in.dst, ty, &val);
            if (strcmp(ty, "f32") == 0) { in.type = TY_F32; in.imm = (int)val; }
            else if (strcmp(ty, "f64") == 0) { in.type = TY_F64; in.imm = (int)val; }
            else { in.type = TY_I32; in.imm = (int)val; }
        } 
        else if (strchr(line, '=')) { // Math and Comparisons
            char op[16], ty[16];
            if (sscanf(line, " %%%d = %15s %15s %%%d %%%d", &in.dst, op, ty, &in.src_args[0], &in.src_args[1]) == 5) {
                if (strcmp(ty, "f32") == 0) in.type = TY_F32;
                else if (strcmp(ty, "f64") == 0) in.type = TY_F64;
                else in.type = TY_I32;

                if (strcmp(op, "add") == 0) in.kind = INS_ADD;
                else if (strcmp(op, "sub") == 0) in.kind = INS_SUB;
                else if (strcmp(op, "mul") == 0) in.kind = INS_MUL;
                else if (strcmp(op, "div") == 0) in.kind = INS_DIV;
                else if (strcmp(op, "mod") == 0) in.kind = INS_MOD;
                else if (strcmp(op, "gt") == 0) in.kind = INS_CMP_GT;
                else if (strcmp(op, "lt") == 0) in.kind = INS_CMP_LT;
                else if (strcmp(op, "eq") == 0) in.kind = INS_CMP_EQ;
            }
        }
        else if (strstr(line, "goto") && !strstr(line, "if_goto")) {
            in.kind = INS_GOTO;
            sscanf(line, " goto %127s", in.label);
        }
        else if (strstr(line, "if_goto")) {
            in.kind = INS_IF_GOTO;
            sscanf(line, " if_goto %%%d %127s", &in.src_args[0], in.label);
        }
        else if (strstr(line, "call")) {
            in.kind = INS_CALL;
            char arg_str[256] = {0};
            if (sscanf(line, " %%%d = call @%127[^( ](%255[^)])", &in.dst, in.label, arg_str) < 2)
                sscanf(line, " call @%127[^( ](%255[^)])", in.label, arg_str);
            char *token = strtok(arg_str, ", %");
            int a = 0;
            while(token && a < 8) { in.src_args[a++] = atoi(token); token = strtok(NULL, ", %"); }
            in.arg_count = a;
        } 
        else if (strstr(line, "puts")) {
            in.kind = INS_PUTS;
            sscanf(line, " puts @%127s", in.label);
        } 
        else if (line[1] == 'L' && strchr(line, ':')) {
            in.kind = INS_LABEL;
            sscanf(line, " %127[^:]", in.label);
        } 
        else if (strstr(line, "ret")) {
            in.kind = INS_RET;
            if (strstr(line, "%")) sscanf(line, " ret %%%d", &in.src_args[0]);
        }

        if (in.kind != 0 || in.dst != -1 || in.label[0] != '\0') {
            add_instr(&funcs[cur], in);
        } else {
            free(in.src_args);
        }
    }
    fclose(f);
}


/* --- Codegen --- */
void emit_arm64() {
    printf("// Generated by EclipseIR\n.data\n");
    // Emit Globals
    for (int i=0; i<global_count; i++) {
        printf("%s: .asciz \"%s\"\n", globals[i].name, globals[i].value);
    }

    printf("\n.text\n.align 2\n");
    for (int f=0; f<func_count; f++) {
        Function *fn = &funcs[f];
        printf(".global %s\n%s:\n", fn->name, fn->name);
        // Prologue: Save Frame Pointer and Link Register
        printf("  stp x29, x30, [sp, #-32]!\n  mov x29, sp\n");

        for (int i=0; i<fn->instr_count; i++) {
            Instr *in = &fn->instrs[i];
            const char* d  = rname(in->dst, in->type);
            const char* s1 = (in->src_args[0] != -1) ? rname(in->src_args[0], in->type) : NULL;
            const char* s2 = (in->src_args[1] != -1) ? rname(in->src_args[1], in->type) : NULL;

            switch (in->kind) {
                case INS_CONST:
                    if (in->type == TY_F32 || in->type == TY_F64) {
                        // For simplicity, we move the bits through a GPR 
                        // In a production compiler, use a literal pool (.rodata)
                        printf("  mov x30, #%d\n  fmov %s, x30\n", in->imm, d);
                    } else {
                        printf("  mov %s, #%d\n", d, in->imm);
                    }
                    break;
                
                
case INS_CMP_GT: case INS_CMP_LT: case INS_CMP_EQ: 
case INS_CMP_GE: case INS_CMP_LE: case INS_CMP_NE: {
    // 1. Comparison sets CPU flags
    if (in->type == TY_I32) printf("  cmp %s, %s\n", s1, s2);
    else printf("  fcmp %s, %s\n", s1, s2);

    // 2. Map IR kind to ARM condition string
    const char *cond;
    switch(in->kind) {
        case INS_CMP_GT: cond = "gt"; break;
        case INS_CMP_LT: cond = "lt"; break;
        case INS_CMP_EQ: cond = "eq"; break;
        case INS_CMP_GE: cond = "ge"; break;
        case INS_CMP_LE: cond = "le"; break;
        case INS_CMP_NE: cond = "ne"; break;
        default: cond = "al"; break;
    }
    // Store boolean result (0 or 1) in destination register
    printf("  cset %s, %s\n", d, cond);
    break;
}

case INS_GOTO:
    printf("  b %s\n", in->label);
    break;

case INS_IF_GOTO:
    // If %src_args[0] is not zero (true), jump to label
    // We use w registers because booleans are TY_I32/TY_BOOL
    printf("  cbnz %s, %s\n", rname(in->src_args[0], TY_I32), in->label);
    break;
    
                    
                case INS_ADD:
                    if (in->type == TY_I32) printf("  add %s, %s, %s\n", d, s1, s2);
                    else printf("  fadd %s, %s, %s\n", d, s1, s2);
                    break;

                case INS_SUB:
                    if (in->type == TY_I32) printf("  sub %s, %s, %s\n", d, s1, s2);
                    else printf("  fsub %s, %s, %s\n", d, s1, s2);
                    break;

                case INS_MUL:
                    if (in->type == TY_I32) printf("  mul %s, %s, %s\n", d, s1, s2);
                    else printf("  fmul %s, %s, %s\n", d, s1, s2);
                    break;

                case INS_DIV:
                    if (in->type == TY_I32) printf("  sdiv %s, %s, %s\n", d, s1, s2);
                    else printf("  fdiv %s, %s, %s\n", d, s1, s2);
                    break;

                case INS_MOD:
                    // Integer Modulo: rem = s1 - (s1 / s2) * s2
                    if (in->type == TY_I32) {
                        printf("  sdiv w30, %s, %s\n", s1, s2);
                        printf("  msub %s, w30, %s, %s\n", d, s2, s1);
                    }
                    break;

                case INS_CALL:
                    // Set up arguments (Standard Calling Convention: w0-w7 or s0-s7)
                    for(int a=0; a < in->arg_count; a++) {
                        // Note: rname logic handles choosing sX vs wX based on type
                        // For simplicity, this assumes i32 args. 
                        // You'd check in->arg_types[a] here in a full compiler.
                        printf("  mov w%d, %s\n", a, rname(in->src_args[a], TY_I32));
                    }
                    printf("  bl %s\n", in->label);
                    if (in->dst != -1) {
                        if (in->type == TY_I32) printf("  mov %s, w0\n", d);
                        else printf("  fmov %s, s0\n", d);
                    }
                    break;

                case INS_PUTS:
                    printf("  adrp x0, %s\n  add x0, x0, :lo12:%s\n  bl puts\n", in->label, in->label);
                    break;

                case INS_LABEL:
                    printf("%s:\n", in->label);
                    break;

                case INS_RET:
                    if (in->src_args[0] != -1) {
                        if (in->type == TY_I32) printf("  mov w0, %s\n", s1);
                        else printf("  fmov s0, %s\n", s1);
                    }
                    // Epilogue: Restore stack and return
                    printf("  ldp x29, x30, [sp], #32\n  ret\n");
                    break;
            }
        }
    }
}



/* --- Memory Cleanup --- */
void cleanup() {
    for (int i = 0; i < func_count; i++) {
        for (int j = 0; j < funcs[i].instr_count; j++) {
            free(funcs[i].instrs[j].src_args);
        }
        free(funcs[i].instrs);
    }
    free(funcs);
    free(globals);
}

/* --- The New CLI --- */
int main(int argc, char **argv) {
    if (argc < 2) {
        printf("Usage: eclipseir <input.ir> [flags]\n");
        printf("Flags:\n  -s <file.s>    Generate assembly file\n");
        printf("  -o <binary>    Generate assembly and compile with gcc\n");
        return 1;
    }

    char *input_file = argv[1];
    char *asm_file = "output.s";
    char *bin_file = NULL;
    int compile = 0;

    // Simple Argument Parser
    for (int i = 2; i < argc; i++) {
        if (strcmp(argv[i], "-s") == 0 && i + 1 < argc) {
            asm_file = argv[++i];
        } else if (strcmp(argv[i], "-o") == 0 && i + 1 < argc) {
            bin_file = argv[++i];
            compile = 1;
        }
    }

    // 1. Parse IR
    parse_ir(input_file);

    // 2. Redirect stdout to the assembly file
    FILE *f = freopen(asm_file, "w", stdout);
    if (!f) { perror("Failed to create assembly file"); return 1; }
    emit_arm64();
    fclose(stdout);

    // Restore stdout for console messages
    freopen("/dev/tty", "w", stdout); 

    // 3. Compile if -o was requested
    if (compile && bin_file) {
        char cmd[512];
        printf("[Building] %s -> %s\n", asm_file, bin_file);
        sprintf(cmd, "gcc %s -o %s", asm_file, bin_file);
        int res = system(cmd);
        if (res == 0) printf("[Success] Binary created: ./%s\n", bin_file);
        else printf("[Error] GCC compilation failed.\n");
    } else {
        printf("[Success] Assembly written to %s\n", asm_file);
    }

    cleanup();
    return 0;
}

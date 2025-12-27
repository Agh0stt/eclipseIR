# srcsetup.py
import subprocess
import os

# 1. Get C source filename
filename = input("Enter the filename of the eclipseIR source (.c file): ").strip()

# 2. Check file existence
if not os.path.isfile(filename):
    print(f"Error: File '{filename}' not found!")
    exit(1)

# 3. Get output executable name
output_name = input("Enter the output executable name (default: same as source): ").strip()
if not output_name:
    output_name = os.path.splitext(filename)[0]

# 4. Check if gcc is available
try:
    subprocess.run(["gcc", "--version"], check=True, stdout=subprocess.DEVNULL)
except (subprocess.CalledProcessError, FileNotFoundError):
    print("Error: gcc is not installed or not in PATH.")
    exit(1)

# 5. Compile the C file
compile_cmd = ["gcc", filename, "-o", output_name]

print(f"Compiling {filename} -> {output_name} ...")
try:
    subprocess.run(compile_cmd, check=True)
    print(f"Compilation successful! Run with ./{output_name}")
except subprocess.CalledProcessError:
    print("Compilation failed!")

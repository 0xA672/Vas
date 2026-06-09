import struct
import sys

def inspect_wasm(path):
    with open(path, "rb") as f:
        magic = f.read(4)
        ver = f.read(4)
        f.seek(0, 2)
        size = f.tell()

    # Validate WASM magic number and version
    wasm_magic = bytes([0x00, 0x61, 0x73, 0x6d])
    wasm_version = bytes([0x01, 0x00, 0x00, 0x00])

    if magic != wasm_magic:
        print(f"Warning: Not a valid WASM module! Magic should be {wasm_magic.hex()}, got {magic.hex()}")
    if ver != wasm_version:
        print(f"Warning: Unknown WASM version! Expected {wasm_version.hex()}, got {ver.hex()}")

    print(f"File:     {path}")
    print(f"Magic:    {magic.hex()}")
    print(f"Version:  {ver.hex()}")
    print(f"Size:     {size} bytes ({size/1024:.1f} KB)")

if __name__ == "__main__":
    path = sys.argv[1] if len(sys.argv) > 1 else "D:/assembler/wasm/vas.wasm"
    inspect_wasm(path)

import struct
with open("D:/assembler/wasm/vas.wasm","rb") as f:
    magic = f.read(4)
    ver = f.read(4)
    print("Magic:", magic.hex())
    print("Version:", ver.hex())
    f.seek(0,2)
    print("Size:", f.tell())

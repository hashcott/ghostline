# AppImage runtime

`runtime-x86_64` is the AppImage type 2 runtime (static, no libfuse2
needed), kept here so every build uses the same bytes.

- Source: https://github.com/AppImage/type2-runtime/releases/tag/20251108
- File: https://github.com/AppImage/type2-runtime/releases/download/20251108/runtime-x86_64
- SHA-256: `2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d`
  (also in `build/linux/Taskfile.yml`, checked before every build)
- License: MIT, Copyright (c) 2004-23 probonopd and contributors

To update: download a newer tagged release, check its SHA-256 against the
one GitHub publishes, replace the file and both hashes.

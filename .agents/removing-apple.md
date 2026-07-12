# Removing Apple Artifacts

When working in this repository, you may come across remnants of Apple Silicon, Darwin, macOS, or Metal support. The project has deliberately dropped support for these platforms.

If you encounter any artifacts related to Apple, Darwin, or Metal (such as `includeDarwin:` blocks, `metal` build targets, `DYLD_LIBRARY_PATH` exports, or macOS specific CI steps), you should handle them as follows:

- **Safely remove them** from configuration files (e.g., `.github/backend-matrix.yml`, `backend/index.yaml`).
- **Remove Darwin-specific CI workflow files** and build steps.
- **Remove Darwin-specific build scripts** or `run.sh` branches.
- **Do not add or restore** any Apple-specific code, documentation, or configurations under any circumstances.
- Ensure that your removal leaves the rest of the file syntactically valid and clean (e.g. no dangling YAML keys or broken shell scripts).

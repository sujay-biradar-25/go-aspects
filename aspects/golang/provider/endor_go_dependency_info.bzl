"""Provider for collecting Go dependency metadata through Bazel aspects."""

EndorGoDependencyInfo = provider(
    doc = "Provider for collecting Go dependency metadata",
    fields = {
        "original_label": "The original target label",
        "name": "Package name or import path",
        "version": "Version string (tag, ref, or 'internal')",
        "dependencies": "List of direct dependency labels",
        "internal": "True if internal workspace target",
        "import_path": "Go import path for the package",
    },
)

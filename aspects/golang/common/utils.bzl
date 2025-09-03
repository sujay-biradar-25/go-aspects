"""Utility functions for Go dependency analysis and version extraction."""

# load("@go_versions_extracted//:versions.bzl", "get_go_version_from_query")
# Fallback function for when go_versions_extracted is not available
def get_go_version_from_query(repo_name):
    """Fallback function to provide a default version when version extraction is not available."""
    return "external"

def _get_external_go_target_details(ctx):
    """Extract name, version, and import path from external Go targets."""
    name = ""
    version = ""
    import_path = ""
    
    label_str = str(ctx.label)
    if not label_str.startswith("@"):
        return name, "external", import_path
    
    repo_name = label_str.split("//")[0][1:]
    
    if hasattr(ctx.rule.attr, "importpath") and ctx.rule.attr.importpath:
        import_path = ctx.rule.attr.importpath
        name = import_path
    
    # Check for explicit version attributes
    if hasattr(ctx.rule.attr, "version") and ctx.rule.attr.version:
        version = ctx.rule.attr.version
        return name, version, import_path
    
    # Check for Git tag/ref attributes
    if hasattr(ctx.rule.attr, "tag") and ctx.rule.attr.tag:
        version = ctx.rule.attr.tag
        return name, version, import_path
    if hasattr(ctx.rule.attr, "ref") and ctx.rule.attr.ref:
        version = ctx.rule.attr.ref
        return name, version, import_path
    
    # Scan for any version-like attributes
    for attr_name in ["version", "tag", "ref", "_version"]:
        if hasattr(ctx.rule.attr, attr_name):
            attr_value = getattr(ctx.rule.attr, attr_name)
            if attr_value and attr_value != "":
                version = attr_value
                return name, version, import_path
    
    # Derive import path from repository name if not found
    if not name:
        if repo_name.startswith("com_github_"):
            name = repo_name.replace("_", "/").replace("com/github/", "github.com/")
        elif repo_name.startswith("org_golang_x_"):
            name = repo_name.replace("_", "/").replace("org/golang/x/", "golang.org/x/")
        else:
            name = repo_name.replace("_", "/")
    
    # Query dynamic version mapping
    version = get_go_version_from_query(repo_name)
    
    return name, version, import_path

def _get_internal_go_target_details(ctx):
    """Extract name, version, and import path from internal Go targets."""
    # Extract target name from the label
    label_parts = str(ctx.label).split(":")
    if len(label_parts) > 1:
        name = label_parts[-1]
    else:
        name = ctx.rule.attr.name if hasattr(ctx.rule.attr, "name") else ""
    
    version = "internal"
    import_path = ""
    
    if hasattr(ctx.rule.attr, "importpath"):
        import_path = ctx.rule.attr.importpath
    if hasattr(ctx.attr, "ref") and ctx.attr.ref:
        version = ctx.attr.ref
    
    return name, version, import_path

def _get_go_name_version_and_import_path(ctx):
    """Route to internal or external target analysis based on label format."""
    is_internal_target = str(ctx.label).startswith("@//") or str(ctx.label).startswith("@@//")
    
    if not is_internal_target:
        return _get_external_go_target_details(ctx)
    
    return _get_internal_go_target_details(ctx)

def _get_go_dependency_labels(deps):
    """Extract label strings from dependency targets."""
    labels = []
    
    for dep in deps:
        if hasattr(dep, "label"):
            labels.append(str(dep.label))
    
    return labels

def _compute_package_version_name(target):
    """Convert a Bazel target label into a safe filename."""
    target_path = target.replace("@//", "_")
    target_path = target_path.replace("@", "")
    target_path = target_path.replace("//:", "_")
    target_path = target_path.replace("/", "_")
    target_path = target_path.replace(":", "_")
    
    if target_path.startswith("_"):
        target_path = target_path[1:]
        
    return target_path

# Public API exports
compute_package_version_name = _compute_package_version_name
get_go_name_version_and_import_path = _get_go_name_version_and_import_path
get_go_dependency_labels = _get_go_dependency_labels
"""Go binary dependency analysis aspects for Bazel 6.5 and rules_go 0.35.0."""

load("//aspects/golang/common:utils.bzl", "compute_package_version_name", "get_go_dependency_labels", "get_go_name_version_and_import_path")
load("//aspects/golang/provider:endor_go_dependency_info.bzl", "EndorGoDependencyInfo")

def _endor_go_binary_resolve_dependencies(target, ctx):
    """Extract dependencies from Go binary targets and create JSON output."""
    if not hasattr(target, "files") and not hasattr(ctx, "attr"):
        return [OutputGroupInfo(endor_sca_info = depset([]))]

    name = ""
    version = ""
    import_path = ""
    deps = []
    
    is_internal_target = str(ctx.label).startswith("@//") or str(ctx.label).startswith("@@//")

    if hasattr(ctx.rule.attr, "deps"):
        deps = ctx.rule.attr.deps

    name, version, import_path = get_go_name_version_and_import_path(ctx)
    dependency_labels = get_go_dependency_labels(deps)

    provider = EndorGoDependencyInfo(
        original_label = str(ctx.label),
        name = name,
        version = version,
        dependencies = dependency_labels,
        internal = is_internal_target,
        import_path = import_path,
    )

    # Create individual node JSON file first
    output_json = ctx.actions.declare_file("pre_merge_{}_resolved_dependencies.json".format(compute_package_version_name(str(ctx.label))))

    # Create JSON content manually using string concatenation to avoid brace escaping issues
    json_content = (
        '{"original_label": "' + provider.original_label.replace('"', '\\"') + 
        '", "name": "' + provider.name.replace('"', '\\"') + 
        '", "version": "' + provider.version.replace('"', '\\"') + 
        '", "dependencies": [' + ", ".join(['"{}"'.format(dep.replace('"', '\\"')) for dep in provider.dependencies]) + 
        '], "internal": ' + ("true" if provider.internal else "false") + 
        ', "import_path": "' + provider.import_path.replace('"', '\\"') + '"}'
    )

    ctx.actions.write(
        output = output_json,
        content = '{"nodes": [' + json_content + ']}',
    )

    # Collect all dependency files to merge
    outputs_to_merge = [output_json]
    for dep in deps:
        if OutputGroupInfo in dep and hasattr(dep[OutputGroupInfo], "endor_sca_info"):
            children = dep[OutputGroupInfo].endor_sca_info.to_list()
            for child in children:
                outputs_to_merge.append(child)

    merged_json = ctx.actions.declare_file("endor_{}_resolved_dependencies.json".format(compute_package_version_name(str(ctx.label))))
    
    # Use Go tool to merge and deduplicate JSON files
    args = ctx.actions.args()
    args.add(merged_json.path)
    args.add_all([f.path for f in outputs_to_merge])

    ctx.actions.run(
        outputs = [merged_json],
        inputs = outputs_to_merge,
        executable = ctx.executable._merge_json_tool,
        arguments = [merged_json.path] + [f.path for f in outputs_to_merge],
        use_default_shell_env = True,
    )

    return [OutputGroupInfo(endor_sca_info = depset([merged_json]))]

internal_endor_go_binary_resolve_dependencies = aspect(
    attr_aspects = ["deps"],
    implementation = _endor_go_binary_resolve_dependencies,
    attrs = {
        "ref": attr.string(default = ""),
        "target_name": attr.string(default = ""),
        "_merge_json_tool": attr.label(
            default = Label("//aspects/golang/common:merge_json_deps"),
            executable = True,
            cfg = "exec",
        ),
    },
)

def _endor_go_binary_get_callgraph_metadata(target, ctx):
    """Placeholder for callgraph metadata collection."""
    return [OutputGroupInfo(endor_sca_info = depset([]))]

internal_endor_go_binary_generate_callgraph_metadata = aspect(
    attr_aspects = ["deps"],
    implementation = _endor_go_binary_get_callgraph_metadata,
    attrs = {
        "ref": attr.string(default = ""),
        "target_name": attr.string(default = ""),
    },
)

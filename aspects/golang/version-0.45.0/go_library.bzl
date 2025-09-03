"""Go library dependency analysis aspects."""

load("//aspects/golang/common:utils.bzl", "compute_package_version_name", "get_go_dependency_labels", "get_go_name_version_and_import_path")
load("//aspects/golang/provider:endor_go_dependency_info.bzl", "EndorGoDependencyInfo")

def _endor_go_library_resolve_dependencies(target, ctx):
    """Extract dependencies from Go library targets and create JSON output."""
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

    output_json = ctx.actions.declare_file("pre_merge_{}_resolved_dependencies.json".format(compute_package_version_name(str(ctx.label))))

    # Manually serialize to JSON
    json_content = '{{"original_label": "{}", "name": "{}", "version": "{}", "dependencies": [{}], "internal": {}, "import_path": "{}"}}'.format(
        provider.original_label.replace('"', '\\"'),
        provider.name.replace('"', '\\"'),
        provider.version.replace('"', '\\"'),
        ", ".join(['"{}"'.format(dep.replace('"', '\\"')) for dep in provider.dependencies]),
        "true" if provider.internal else "false",
        provider.import_path.replace('"', '\\"'),
    )

    ctx.actions.write(
        output = output_json,
        content = "{\"nodes\": [" + json_content + "]}",
    )

    outputs_to_merge = [output_json]
    for dep in deps:
        if OutputGroupInfo in dep and hasattr(dep[OutputGroupInfo], "endor_sca_info"):
            children = dep[OutputGroupInfo].endor_sca_info.to_list()
            for child in children:
                outputs_to_merge.append(child)

    merged_json = ctx.actions.declare_file("endor_{}_resolved_dependencies.json".format(compute_package_version_name(str(ctx.label))))
    
    # Use Go tool to merge and deduplicate JSON files
    ctx.actions.run(
        outputs = [merged_json],
        inputs = outputs_to_merge,
        executable = ctx.executable._merge_json_tool,
        arguments = [merged_json.path] + [f.path for f in outputs_to_merge],
        use_default_shell_env = True,
    )

    return [OutputGroupInfo(endor_sca_info = depset([merged_json]))]

internal_endor_go_library_resolve_dependencies = aspect(
    attr_aspects = ["deps"],
    implementation = _endor_go_library_resolve_dependencies,
    attrs = {
        "ref": attr.string(),
        "target_name": attr.string(),
        "_merge_json_tool": attr.label(
            default = Label("//aspects/golang/common:merge_json_deps"),
            executable = True,
            cfg = "exec",
        ),
    },
)

def _endor_go_library_get_callgraph_metadata(target, ctx):
    """Extract callgraph metadata from Go library targets using gopackagesdriver and VTA."""
    if not hasattr(target, "files") and not hasattr(ctx, "attr"):
        return [OutputGroupInfo(endor_callgraph_info = depset([]))]

    # Only process go_library targets
    if ctx.rule.kind != "go_library":
        return [OutputGroupInfo(endor_callgraph_info = depset([]))]

    name, version, import_path = get_go_name_version_and_import_path(ctx)
    
    if not import_path:
        # Skip targets without import paths
        return [OutputGroupInfo(endor_callgraph_info = depset([]))]
    
    # Create callgraph analysis output file
    callgraph_json = ctx.actions.declare_file("callgraph_{}.json".format(compute_package_version_name(str(ctx.label))))
    
    # Get the Go source files for this target
    go_sources = []
    if hasattr(ctx.rule.files, "srcs"):
        go_sources = [f for f in ctx.rule.files.srcs if f.path.endswith(".go")]
    
    if not go_sources:
        # No Go source files to analyze
        ctx.actions.write(
            output = callgraph_json,
            content = '{"target": "' + str(ctx.label) + '", "import_path": "' + import_path + '", "callgraph": [], "error": "No Go source files found"}',
        )
        return [OutputGroupInfo(endor_callgraph_info = depset([callgraph_json]))]
    
    # Use the callgraph analyzer with VTA and packages.Load
    args = ctx.actions.args()
    args.add(str(ctx.label))  # target
    args.add(import_path)     # import_path
    args.add(callgraph_json.path)  # output_file
    
    # Add all source files as arguments
    for src_file in go_sources:
        args.add(src_file.path)
    
    # Set up environment with GOPACKAGESDRIVER pointing to Bazel's gopackagesdriver
    env = {}
    env.update(ctx.configuration.default_shell_env)
    env["GOPACKAGESDRIVER"] = ctx.executable._gopackagesdriver.path
    
    ctx.actions.run(
        outputs = [callgraph_json],
        inputs = go_sources + [ctx.executable._gopackagesdriver],
        executable = ctx.executable._protobuf_analyzer,
        arguments = [args],
        env = env,
    )
    
    return [OutputGroupInfo(endor_callgraph_info = depset([callgraph_json]))]

internal_endor_go_library_generate_callgraph_metadata = aspect(
    attr_aspects = ["deps"],
    implementation = _endor_go_library_get_callgraph_metadata,
    attrs = {
        "ref": attr.string(),
        "target_name": attr.string(),
        "_protobuf_analyzer": attr.label(
            default = Label("//tools/callgraph:callgraph"),
            executable = True,
            cfg = "exec",
        ),
        "_gopackagesdriver": attr.label(
            default = Label("@rules_go//go/tools/gopackagesdriver"),
            executable = True,
            cfg = "exec",
        ),
    },
)

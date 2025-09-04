"""Go binary dependency analysis aspects."""

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
        "ref": attr.string(values = [""]),
        "target_name": attr.string(values = [""]),
        "_merge_json_tool": attr.label(
            default = Label("//aspects/golang/common:merge_json_deps"),
            executable = True,
            cfg = "exec",
        ),
    },
)

def _generate_packages_json(target, ctx, source_files):
    """Generate packages JSON directly from Bazel aspect context, with HARDCODED stdlib support."""
    
    # Extract package information from target
    package_label = str(ctx.label)
    package_name = ctx.label.name
    package_path = ctx.label.package or package_name
    
    # Collect Go files from target
    go_files = []
    compiled_go_files = []
    
    for src_file in source_files:
        if src_file.extension == "go":
            # Use full workspace path instead of relative path
            file_path = "/Users/sbiradar/code/aspects-test/golang/" + src_file.path
            go_files.append(file_path)
            compiled_go_files.append(file_path)
    
    # Try to find the export file (.x file) for type information
    export_file = ""
    if hasattr(target, "go"):
        go_info = target[DefaultInfo]
        for output in go_info.files.to_list():
            if output.extension == "x":
                export_file = output.path
                break
    
    # HARDCODE standard library packages with their proper metadata
    stdlib_packages = {
        "fmt": {
            "ID": "fmt",
            "Name": "fmt", 
            "PkgPath": "fmt",
            "ExportFile": "__BAZEL_EXECROOT__/external/go_sdk/pkg/darwin_arm64/fmt.a",
            "GoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/fmt/print.go"],
            "CompiledGoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/fmt/print.go"],
            "Imports": {"errors": "errors", "io": "io", "os": "os", "reflect": "reflect", "strconv": "strconv", "sync": "sync", "unicode/utf8": "unicode/utf8"},
            "Types": {},
            "TypesInfo": {},
            "TypesSizes": {},
            "Syntax": []
        },
        "context": {
            "ID": "context",
            "Name": "context",
            "PkgPath": "context", 
            "ExportFile": "__BAZEL_EXECROOT__/external/go_sdk/pkg/darwin_arm64/context.a",
            "GoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/context/context.go"],
            "CompiledGoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/context/context.go"],
            "Imports": {"errors": "errors", "fmt": "fmt", "reflect": "reflect", "sort": "sort", "sync": "sync", "time": "time"},
            "Types": {},
            "TypesInfo": {},
            "TypesSizes": {},
            "Syntax": []
        },
        "os": {
            "ID": "os",
            "Name": "os",
            "PkgPath": "os",
            "ExportFile": "__BAZEL_EXECROOT__/external/go_sdk/pkg/darwin_arm64/os.a",
            "GoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/os/file.go"],
            "CompiledGoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/os/file.go"],
            "Imports": {"errors": "errors", "io": "io", "runtime": "runtime", "sync": "sync", "syscall": "syscall", "time": "time"},
            "Types": {},
            "TypesInfo": {},
            "TypesSizes": {},
            "Syntax": []
        },
        "time": {
            "ID": "time",
            "Name": "time",
            "PkgPath": "time",
            "ExportFile": "__BAZEL_EXECROOT__/external/go_sdk/pkg/darwin_arm64/time.a", 
            "GoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/time/time.go"],
            "CompiledGoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/time/time.go"],
            "Imports": {"errors": "errors", "runtime": "runtime", "sync": "sync", "syscall": "syscall"},
            "Types": {},
            "TypesInfo": {},
            "TypesSizes": {},
            "Syntax": []
        },
        "net/http": {
            "ID": "net/http",
            "Name": "http",
            "PkgPath": "net/http",
            "ExportFile": "__BAZEL_EXECROOT__/external/go_sdk/pkg/darwin_arm64/net/http.a",
            "GoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/net/http/client.go"],
            "CompiledGoFiles": ["__BAZEL_EXECROOT__/external/go_sdk/src/net/http/client.go"],
            "Imports": {"bufio": "bufio", "context": "context", "crypto/tls": "crypto/tls", "errors": "errors", "fmt": "fmt", "io": "io", "net": "net", "net/url": "net/url", "sort": "sort", "strconv": "strconv", "strings": "strings", "sync": "sync", "time": "time"},
            "Types": {},
            "TypesInfo": {},
            "TypesSizes": {},
            "Syntax": []
        }
    }
    
    # Extract import information from dependencies
    imports = {}
    dependency_packages = []
    
    if hasattr(ctx.rule.attr, "deps") and ctx.rule.attr.deps:
        for dep in ctx.rule.attr.deps:
            dep_label = str(dep.label)
            
            # Map dependency labels to import paths
            if dep_label.startswith("@rules_go//stdlib:"):
                import_name = dep_label.split(":")[-1]
                imports[import_name] = dep_label
            elif dep_label.startswith("@//"):
                # Internal dependencies
                import_path = dep_label[3:].replace(":", "/")
                imports[import_path] = dep_label
            elif dep_label.startswith("@"):
                # External dependencies  
                import_name = dep_label.split("/")[-1].split(":")[-1]
                imports[import_name] = dep_label
            
            # Add dependency package info (simplified for VTA)
            dep_package = {
                "ID": dep_label,
                "Name": dep_label.split(":")[-1],
                "PkgPath": dep_label.split(":")[-1],
                "GoFiles": [],
                "CompiledGoFiles": [],
                "Imports": {},
                "ExportFile": "",  # Would need to traverse to get actual export file
            }
            dependency_packages.append(dep_package)
    
    # Build the main package entry
    main_package = {
        "ID": package_label,
        "Name": package_name,
        "PkgPath": package_path,
        "GoFiles": go_files,
        "CompiledGoFiles": compiled_go_files,
        "Imports": imports,
        "ExportFile": export_file,  # Include type information
    }
    
    # Create all packages (main + dependencies + HARDCODED STDLIB)
    stdlib_package_list = [stdlib_packages[pkg] for pkg in stdlib_packages]
    all_packages = [main_package] + dependency_packages + stdlib_package_list
    
    # Create the response structure that VTA analyzer expects WITH STDLIB
    response = {
        "NotHandled": False,
        "Compiler": "gc", 
        "Arch": "arm64",  # Could be detected from ctx if needed
        "Roots": [package_label],
        "Packages": all_packages
    }
    
    # Convert to JSON string for embedding in shell script
    return json.encode(response)

def _create_package_info_with_sources(ctx):
    """Create package information for VTA analysis using workspace source files."""
    
    # Extract the main package information
    name, version, import_path = get_go_name_version_and_import_path(ctx)
    
    # Get the actual package path from the Bazel label
    package_path = ctx.label.package
    
    # Use the package path directly for the PkgPath (this is what packages.Load expects)
    if not import_path:
        import_path = package_path
    
    # Create package info that the VTA analyzer can use
    # The VTA analyzer will handle the package loading itself
    main_package = {
        "ID": str(ctx.label),
        "Name": name or "main",
        "PkgPath": package_path,  # Use the Bazel package path directly
        "GoFiles": [],  # Let the VTA analyzer discover the files
        "CompiledGoFiles": [],
        "Imports": {},
        "ExportFile": "",
    }
    
    # Collect all Go dependencies from the Bazel dependency graph
    all_packages = [main_package]
    
    # Helper function to extract Go package info from a target
    def extract_go_package_info(target, target_label):
        if hasattr(target, "files"):
            # Check if any files end with .go
            has_go_files = False
            for f in target.files.to_list():
                if f.path.endswith(".go"):
                    has_go_files = True
                    break
            
            if has_go_files:
                pkg_path = target_label.package
                return {
                    "ID": str(target_label),
                    "Name": target_label.name,
                    "PkgPath": pkg_path,
                    "GoFiles": [],
                    "CompiledGoFiles": [],
                    "Imports": {},
                    "ExportFile": "",
                }
        return None
    
    # Collect dependencies from embed attribute (go_library targets)
    if hasattr(ctx.rule.attr, "embed"):
        for embed_dep in ctx.rule.attr.embed:
            # Add the embedded library itself
            pkg_info = extract_go_package_info(embed_dep, embed_dep.label)
            if pkg_info:
                all_packages.append(pkg_info)
            
            # Also collect dependencies of the embedded library through the rule attribute
            if hasattr(embed_dep, "attr") and hasattr(embed_dep.attr, "deps"):
                for nested_dep in embed_dep.attr.deps:
                    nested_pkg_info = extract_go_package_info(nested_dep, nested_dep.label)
                    if nested_pkg_info:
                        all_packages.append(nested_pkg_info)
    
    # Collect dependencies from direct deps attribute
    if hasattr(ctx.rule.attr, "deps"):
        for dep in ctx.rule.attr.deps:
            pkg_info = extract_go_package_info(dep, dep.label)
            if pkg_info:
                all_packages.append(pkg_info)
    
    # TODO: Recursively collect transitive dependencies
    # For now, we'll let the VTA analyzer discover packages through Go's module system
    
    # Create the response structure with dynamic values
    # The VTA analyzer will detect architecture at runtime
    response = {
        "NotHandled": False,
        "Compiler": "gc",
        "Arch": "auto-detect",  # Will be detected by VTA analyzer
        "Roots": [str(ctx.label)],
        "Packages": all_packages
    }
    
    return json.encode(response)

# Removed hardcoded stdlib packages - VTA analyzer will discover them dynamically
def _endor_go_binary_get_callgraph_metadata(target, ctx):
    """Extract callgraph metadata from Go binary targets using VTA analyzer."""
    if not hasattr(target, "files") and not hasattr(ctx, "attr"):
        return [OutputGroupInfo(endor_callgraph_info = depset([]))]

    # Only process go_binary targets
    if ctx.rule.kind != "go_binary":
        return [OutputGroupInfo(endor_callgraph_info = depset([]))]

    name, version, import_path = get_go_name_version_and_import_path(ctx)
    
    # Create callgraph analysis output file
    callgraph_json = ctx.actions.declare_file("callgraph_{}.json".format(compute_package_version_name(str(ctx.label))))
    
    # Create package information for VTA analysis with actual source files
    # We construct paths to the workspace source files for VTA analysis
    packages_json_content = _create_package_info_with_sources(ctx)
    
    # Create packages JSON file
    packages_json_file = ctx.actions.declare_file("packages_{}.json".format(compute_package_version_name(str(ctx.label))))
    ctx.actions.write(
        output = packages_json_file,
        content = packages_json_content,
    )
    
    # Collect actual source files to pass as inputs
    source_files = []
    
    # Get source files from embedded libraries
    if hasattr(ctx.rule.attr, "embed"):
        for embed_dep in ctx.rule.attr.embed:
            if DefaultInfo in embed_dep:
                embed_files = embed_dep[DefaultInfo].files.to_list()
                go_files = [f for f in embed_files if f.path.endswith(".go")]
                source_files.extend(go_files)
    
    # Get source files from direct srcs
    if hasattr(ctx.rule.files, "srcs"):
        target_sources = [f for f in ctx.rule.files.srcs if f.path.endswith(".go")]
        source_files.extend(target_sources)
    
    # Run VTA analyzer tool with actual source files as inputs
    args = ctx.actions.args()
    args.add(packages_json_file.path)
    args.add(callgraph_json.path)
    
    ctx.actions.run(
        outputs = [callgraph_json],
        inputs = [packages_json_file] + source_files,
        executable = ctx.executable._vta_analyzer_tool,
        arguments = [args],
        use_default_shell_env = True,
        mnemonic = "VTACallGraphAnalysis",
        progress_message = "Analyzing call graph for %s" % ctx.label,
    )
    
    return [OutputGroupInfo(endor_callgraph_info = depset([callgraph_json]))]

internal_endor_go_binary_generate_callgraph_metadata = aspect(
    attr_aspects = ["deps"],
    implementation = _endor_go_binary_get_callgraph_metadata,
    attrs = {
        "ref": attr.string(default = ""),
        "target_name": attr.string(default = ""),
        "_vta_analyzer_tool": attr.label(
            default = Label("//aspects/golang/common:vta_analyzer_simple"),
            executable = True,
            cfg = "exec",
        ),
    },
)

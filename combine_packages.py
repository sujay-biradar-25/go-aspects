#!/usr/bin/env python3

import json
import subprocess
import os
import sys

def get_external_packages_from_bazel():
    """Extract external package information from Bazel query"""
    try:
        # Get external Go dependencies
        cmd = ["bazel", "query", "deps(//src/main:main)", "--output=label"]
        result = subprocess.run(cmd, capture_output=True, text=True, cwd="/Users/sbiradar/code/go-aspects")

        if result.returncode != 0:
            print(f"Error running bazel query: {result.stderr}")
            return []

        external_packages = []
        for line in result.stdout.strip().split('\n'):
            line = line.strip()
            if line.startswith('@') and 'go_default_library' in line:
                # Parse external dependency
                # Format: @com_github_google_uuid//:go_default_library
                parts = line.split('//')
                if len(parts) >= 2:
                    repo_name = parts[0][1:]  # Remove @

                    # Convert Bazel repo name to Go import path
                    import_path = convert_bazel_to_import_path(repo_name)
                    if import_path:
                        external_packages.append({
                            "ID": import_path,
                            "Name": get_package_name_from_path(import_path),
                            "PkgPath": import_path,
                            "GoFiles": [],
                            "CompiledGoFiles": [],
                            "Imports": {},
                            "BazelTarget": line
                        })

        return external_packages
    except Exception as e:
        print(f"Error extracting external packages: {e}")
        return []

def convert_bazel_to_import_path(bazel_repo_name):
    """Convert Bazel repository name to Go import path"""
    conversions = {
        'com_github_google_uuid': 'github.com/google/uuid',
        'com_github_gorilla_mux': 'github.com/gorilla/mux',
        'com_github_sirupsen_logrus': 'github.com/sirupsen/logrus',
        'org_golang_x_time': 'golang.org/x/time/rate',
        'com_github_gin_gonic_gin': 'github.com/gin-gonic/gin',
        'com_github_go_redis_redis_v8': 'github.com/go-redis/redis/v8',
        'com_github_golang_jwt_jwt_v4': 'github.com/golang-jwt/jwt/v4',
        'com_github_prometheus_client_golang': 'github.com/prometheus/client_golang',
        'org_golang_x_crypto': 'golang.org/x/crypto'
    }

    return conversions.get(bazel_repo_name)

def get_package_name_from_path(import_path):
    """Extract package name from import path"""
    return import_path.split('/')[-1]

def load_gopackagesdriver_output():
    """Load the existing gopackagesdriver output"""
    try:
        with open('/Users/sbiradar/code/go-aspects/gopackages_analysis_output.json', 'r') as f:
            content = f.read()
            # Find the JSON part (skip Bazel output at the beginning)
            json_start = content.find('{"NotHandled"')
            if json_start == -1:
                json_start = content.find('{')

            if json_start != -1:
                json_content = content[json_start:]
                return json.loads(json_content)
    except Exception as e:
        print(f"Error loading gopackagesdriver output: {e}")

    return None

def combine_packages():
    """Combine stdlib packages from gopackagesdriver with external packages from Bazel"""

    print("🔄 Loading gopackagesdriver output...")
    stdlib_data = load_gopackagesdriver_output()

    if not stdlib_data:
        print("❌ Failed to load gopackagesdriver output")
        return

    print(f"✅ Loaded {len(stdlib_data.get('Packages', []))} stdlib packages")

    print("🔄 Extracting external packages from Bazel...")
    external_packages = get_external_packages_from_bazel()

    if not external_packages:
        print("⚠️  No external packages found")
    else:
        print(f"✅ Found {len(external_packages)} external packages")

    # Combine the packages
    combined_packages = stdlib_data.get('Packages', []) + external_packages

    # Create combined output
    combined_output = {
        "NotHandled": False,
        "Compiler": stdlib_data.get('Compiler', 'gc'),
        "Arch": stdlib_data.get('Arch', 'arm64'),
        "Roots": stdlib_data.get('Roots', []) + [pkg['ID'] for pkg in external_packages],
        "Packages": combined_packages,
        "GoVersion": stdlib_data.get('GoVersion', 0),
        "Summary": {
            "TotalPackages": len(combined_packages),
            "StdlibPackages": len(stdlib_data.get('Packages', [])),
            "ExternalPackages": len(external_packages),
            "ExternalPackagesList": [pkg['PkgPath'] for pkg in external_packages]
        }
    }

    # Write combined output
    output_file = '/Users/sbiradar/code/go-aspects/combined_packages_analysis.json'
    with open(output_file, 'w') as f:
        json.dump(combined_output, f, indent=2)

    print(f"✅ Combined analysis saved to: {output_file}")
    print(f"📊 Total packages: {len(combined_packages)}")
    print(f"📚 Stdlib packages: {len(stdlib_data.get('Packages', []))}")
    print(f"🔗 External packages: {len(external_packages)}")

    if external_packages:
        print("\n🔗 External packages found:")
        for pkg in external_packages:
            print(f"   - {pkg['PkgPath']}")

if __name__ == "__main__":
    combine_packages()

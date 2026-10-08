#!/bin/sh
set -eu

gopls_binary=${1:-gopls}
export GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=live"

temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/sber-gopls.XXXXXX")
cleanup() {
    for artifact in files diagnostics; do
        if [ -f "$temporary_directory/$artifact" ]; then
            rm "$temporary_directory/$artifact"
        fi
    done
    rmdir "$temporary_directory"
}
trap cleanup 0

# Include source and test files for the current platform without running tests.
go list -f '{{range .GoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .CgoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .TestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}{{range .XTestGoFiles}}{{$.Dir}}/{{.}}{{"\n"}}{{end}}' ./... > "$temporary_directory/files"

set --
while IFS= read -r file; do
    [ -z "$file" ] || set -- "$@" "$file"
done < "$temporary_directory/files"

if [ "$#" -eq 0 ]; then
    printf '%s\n' 'No Go files were found for gopls check.' >&2
    exit 1
fi

printf 'Check %s Go files with gopls.\n' "$#"
# gopls can return zero after it prints diagnostics. Fail on that output, too.
if "$gopls_binary" check -severity=warning "$@" > "$temporary_directory/diagnostics" 2>&1 &&
    [ ! -s "$temporary_directory/diagnostics" ]; then
    exit 0
fi

cat "$temporary_directory/diagnostics"
exit 1

#!/bin/bash
# Generate seccomp profile from strace log

set -e

STRACE_LOG="${1:-strace.log}"
OUTPUT_FILE="${2:-seccomp-profile.json}"

if [ ! -f "$STRACE_LOG" ]; then
    echo "Error: strace log file '$STRACE_LOG' not found"
    exit 1
fi

echo "Parsing strace log: $STRACE_LOG"

# Extract unique syscalls from strace output
# Pattern: PID spaces syscall_name(
# Using sed to extract syscall names (works on macOS and Linux)
SYSCALLS=$(sed -n 's/^[0-9][0-9]*[[:space:]]*\([a-z_][a-z_0-9]*\)(.*/\1/p' "$STRACE_LOG" | sort -u)

# Count syscalls
SYSCALL_COUNT=$(echo "$SYSCALLS" | grep -v '^$' | wc -l | tr -d ' ')

echo "Found $SYSCALL_COUNT unique syscalls"

# Generate JSON array of syscalls
SYSCALL_JSON=$(echo "$SYSCALLS" | grep -v '^$' | sed 's/^/        "/' | sed 's/$/",/' | sed '$ s/,$//')

# Generate seccomp profile JSON
cat > "$OUTPUT_FILE" <<EOF
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": [
    "SCMP_ARCH_X86_64",
    "SCMP_ARCH_X86",
    "SCMP_ARCH_X32"
  ],
  "syscalls": [
    {
      "names": [
$SYSCALL_JSON
      ],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
EOF

echo "✅ Seccomp profile generated: $OUTPUT_FILE"
echo "   Contains $SYSCALL_COUNT syscalls"


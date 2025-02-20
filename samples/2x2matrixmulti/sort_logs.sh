#!/bin/bash

if [ "$#" -lt 1 ]; then
    echo "Usage: $0 <log_file> [output_file]"
    exit 1
fi

LOG_FILE="$1"
OUTPUT_FILE="${2:-sorted.log}"  # Default to "sorted.log" if not provided.

if [ ! -f "$LOG_FILE" ]; then
    echo "Error: Log file '$LOG_FILE' not found."
    exit 1
fi

# Extract tile identifiers and sort them
CORES=$(grep -Eo 'Tile\[[0-9]+\]\[[0-9]+\]' "$LOG_FILE" | sort -t'[' -k2,2n -k3,3n | uniq)


# Clear the output file
> "$OUTPUT_FILE"

while read -r CORE; do
  echo "===== $CORE =====" >> "$OUTPUT_FILE"
  
  # Escape square brackets for regex usage.
  escaped_core=$(echo "$CORE" | sed 's/\[/\\[/g; s/\]/\\]/g')
  
  # Build a regex that matches lines that start with a timestamp,
  # a comma, optional whitespace, and then "Device.<Tile>.Core"
  regex="^[[:space:]]*[0-9]+\.[0-9]+,\s*Device\.${escaped_core}\.Core\b"
  
  # Only include lines where the primary device field is exactly the tile.
  grep -E "$regex" "$LOG_FILE" >> "$OUTPUT_FILE"
  echo "" >> "$OUTPUT_FILE"
done <<< "$CORES"

# echo "===== Numeric Matrix Outputs =====" >> "$OUTPUT_FILE"
# grep -E "\[[0-9]+( [0-9]+){3}\]" "$LOG_FILE" >> "$OUTPUT_FILE"

echo "===== Matrix Multiplication Output =====" >> "$OUTPUT_FILE"
grep -E "^\[[0-9]+( [0-9]+)*\] \* \[[0-9]+( [0-9]+)*\] = \[[0-9]+( [0-9]+)*\]$" "$LOG_FILE" >> "$OUTPUT_FILE"

grep -E 'Device\.Tile\[[0-9]+\]\[[0-9]+\]\.Core, (Send|Recv)' "$LOG_FILE" >> "check_sequence.log"
echo "Sorted logs saved to $OUTPUT_FILE"
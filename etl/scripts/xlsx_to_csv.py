#!/usr/bin/env python3
"""Convert a Census xlsx delineation file to CSV.

The Census Bureau publishes the CBSA delineation file (list1_<vintage>.xlsx)
in Excel format only — no CSV variant. This single-purpose script converts
it so the Go ETL pipeline (api/internal/etl/us) can read it.

Usage:
    python3 etl/scripts/xlsx_to_csv.py <input.xlsx> <output.csv>

Dependencies:
    openpyxl, pinned in mise.toml under [tools] as "pipx:openpyxl".
    After `mise install` the script is invocable from any cwd.

Deterministic: same input xlsx produces byte-identical CSV output
(matching the rest of the ETL pipeline's reproducibility contract).
"""

import csv
import sys

import openpyxl  # type: ignore[import-not-found]


def main(argv: list[str]) -> int:
    if len(argv) != 3:
        print(f"usage: {argv[0]} <input.xlsx> <output.csv>", file=sys.stderr)
        return 2
    in_path, out_path = argv[1], argv[2]

    wb = openpyxl.load_workbook(in_path, read_only=True, data_only=True)
    ws = wb.active
    if ws is None:
        print(f"xlsx_to_csv: {in_path} has no active sheet", file=sys.stderr)
        return 1

    with open(out_path, "w", newline="") as f:
        writer = csv.writer(f)
        for row in ws.iter_rows(values_only=True):
            writer.writerow(["" if v is None else v for v in row])
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

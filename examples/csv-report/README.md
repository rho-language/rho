# csv-report

A command-line tool written in Rho that summarises a CSV file and, if you ask,
groups its rows by one column.

```
csv-report <input.csv> [--group-by <column>] [--sum <column>]
```

- Without options it prints the row count, the column names, and the minimum,
  maximum and mean of every numeric column. The mean has two decimals.
- `--group-by region` prints one line for each distinct value of `region`, with
  how many rows had it.
- `--group-by region --sum units` also adds up `units` for each group, and sorts
  the groups by that sum, largest first.

The file needs a header row. Fields are separated by commas. A field in double
quotes may contain commas, and `""` inside quotes stands for one `"`. Blank
lines are skipped, and CRLF line endings work.

A malformed file stops the program with exit status 1 and a message on
standard error that gives the line number:

```
csv-report: tests/data/unterminated.csv: line 3: unterminated quoted field 2
csv-report: tests/data/short_row.csv: line 3: expected 2 fields, found 1
csv-report: tests/data/bad_number.csv: line 3: "abc" in column v is not a number
```

A field that is a CSV record split across lines (a quoted field containing a
newline) counts as an unterminated quote.

## Build

```sh
rho build .
```

This builds both targets that `module.yml` declares:

| Target | Program |
|---|---|
| Go | `build/csv_report` |
| C  | `build/output/csv_report` |

To build only one of them, put the flag before the directory:
`rho build --target go .` (`rho build . --target go` is refused).

## Run

```sh
./build/output/csv_report sample.csv --group-by region --sum units
```

```
rows: 20
columns (6): date region product units price customer
  units: min 1, max 30, mean 7.40
  price: min -1.5, max 149, mean 43.46

by region:
  North: count 6, sum of units 45.00
  East: count 5, sum of units 44.00
  South: count 5, sum of units 35.00
  West: count 4, sum of units 24.00
```

`rho run .` cannot pass arguments to the program, so run the built binary
directly. The Go and the C program print the same output.

The program reads its arguments from `/proc/self/cmdline` because the C
backend has no built-in for the command line. That means it runs on Linux
only.

## Test

```sh
rho test
```

`tests/tests.yml` declares three suites:

- `tests/csv_test.rho` tests the record parser: plain, empty, quoted and
  escaped fields, CRLF, and malformed records.
- `tests/report_test.rho` tests number formatting and rounding, numeric-column
  detection, grouping and sorting, the whole report for a small document, and
  the line numbers in error messages.
- `tests/parity.sh` builds both programs and checks that they print the same
  output and exit with the same status for the sample and for each malformed
  file in `tests/data/`.

The Rho test files are always compiled through the C backend. Running
`rho test` replaces `build/csv_report` with a test program, so run `rho build .`
again before you use the Go binary.

## Layout

| File | What it holds |
|---|---|
| `src/csv.rho` | `parse_record`: splits one line into fields |
| `src/report.rho` | column statistics, grouping, sorting, and the report text |
| `src/main.rho` | arguments, reading the file in buffers, error messages, exit status |
| `sample.csv` | 20 rows of sales data, with quoted fields that contain commas and quotes |
| `FINDINGS.md` | what got in the way while writing this example from the documentation |

Numbers are stored as thousandths in an `i64` (`milli_parse`), so a value
needs at most three decimals; any further digits are dropped. The program
reads at most 32 columns, 256 distinct `--group-by` values and 64 KiB per line.

import os
import re
import sys

STEP = 10
WIDTH = 6

TIMESTAMP_RE = re.compile(r"^(\d{14})_(.+)$")
PREFIX_RE = re.compile(r"^(\d{6})_")


def get_existing_max_prefix(files: list[str], directory: str) -> int:
    """Return the highest numeric prefix among already-prefixed files."""
    max_prefix = 0
    for f in files:
        if not os.path.isfile(os.path.join(directory, f)):
            continue
        m = PREFIX_RE.match(f)
        if m:
            max_prefix = max(max_prefix, int(m.group(1)))
    return max_prefix


def get_timestamp_files(files: list[str], directory: str) -> list[str]:
    """Return list of timestamp-prefixed migration files."""
    ts_files: list[str] = []
    for f in files:
        if not os.path.isfile(os.path.join(directory, f)):
            continue
        if TIMESTAMP_RE.match(f):
            ts_files.append(f)
    return ts_files


def main(directory: str) -> None:
    if not os.path.isdir(directory):
        print(f"Directory not found: {directory}")
        sys.exit(1)

    files = sorted(os.listdir(directory))

    timestamp_files = get_timestamp_files(files, directory)
    if not timestamp_files:
        print("No timestamp-prefixed files found.")
        return

    counter = get_existing_max_prefix(files, directory) + STEP

    for old_name in timestamp_files:
        old_path = os.path.join(directory, old_name)
        match = TIMESTAMP_RE.match(old_name)
        if match is None:
            print(f"The file is not timestamp-prefixed, skipped the file: {old_name}")
            return
        _, name_part = match.groups()

        new_name = f"{counter:0{WIDTH}d}_{name_part}"
        new_path = os.path.join(directory, new_name)

        print(f"{old_name} → {new_name}")
        os.rename(old_path, new_path)

        counter += STEP


if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python fix_ts_migrations.py <directory>")
        sys.exit(1)

    main(sys.argv[1])

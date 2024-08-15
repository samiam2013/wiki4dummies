#!/usr/bin/python3

import mwparserfromhell
import sys

# read the first argument as the output file
# output = sys.argv[1]

input = ""
for line in sys.stdin:
    input += line + "\n"


wikicode = mwparserfromhell.parse(input)
stripped = wikicode.strip_code()
print(stripped)
# write the output to the file

# with open(output, "w") as f:
#     f.write(stripped)

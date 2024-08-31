#!/usr/bin/python3

import mwparserfromhell
import sys

input = ""
for line in sys.stdin:
    input += line + "\n"

wikicode = mwparserfromhell.parse(input)
stripped = wikicode.strip_code()
print(stripped)

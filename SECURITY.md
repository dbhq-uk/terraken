# Security

## Reporting a vulnerability

Email dan@dbhq.uk. Say what you found, how to reproduce it, and what you
think the impact is.

You will get an acknowledgement within three working days. Please give a
reasonable window for a fix before making it public.

## What this tool touches

terraverdict reads one file, or one stream, and writes a report. It never
runs `terraform`, never reads cloud credentials, never makes a network call,
never applies anything and never writes a file.

The `--format html` report is a single self-contained document: inline CSS,
no external stylesheet, no font, no image and no script. Every value taken
from the plan is HTML-escaped before it is written, because a resource
address can carry a `for_each` key chosen by whoever wrote the Terraform -
on a fork pull request, that is not someone you trust - and the report is
opened by a reviewer.

## Plan files are secrets

The sensitive part is the input, not the tool. A `terraform show -json` plan
can contain credentials in the clear whether or not Terraform marked them
`sensitive` - the README has a real example found while building this.

terraverdict never prints an attribute's value, marked or not. It names the
path and stops. `tv -` reads the plan from standard input, so it never has to
be written to disk at all.

If you find a case where a value does reach the output, that is a
vulnerability in this tool. Report it by the route above.

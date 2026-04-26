# Real-App Smoke Testing

Use this before public releases to keep `docs/support-matrix.md` honest.

The smoke script builds `doppel` if needed, clones a list of installed apps into
a temporary directory, writes one JSON report per app, and prints a Markdown
summary table. When launch testing is enabled, `jq` is required so the script
can require `verify.launch_test.survived == true` before marking an app as
passing.

```bash
scripts/smoke-real-apps.sh
```

Default app candidates:

- `/Applications/cmux.app`
- `/Applications/Alacritty.app`
- `/Applications/Ghostty.app`
- `/Applications/LocalSend.app`
- `/Applications/Cherry Studio.app`

To test a custom set:

```bash
scripts/smoke-real-apps.sh /Applications/Ghostty.app "/Applications/Cherry Studio.app"
```

By default the script runs `--launch-test`, which briefly starts each clone and
then terminates it. Disable that when you only want clone/verify coverage:

```bash
DOPPEL_SMOKE_LAUNCH_TEST=0 scripts/smoke-real-apps.sh
```

Outputs go to `/tmp/doppel-smoke` by default. Override with:

```bash
DOPPEL_SMOKE_DIR=/tmp/my-doppel-smoke scripts/smoke-real-apps.sh
```

After a run, update `docs/support-matrix.md` from the JSON reports. Do not mark
an app as "Launches" unless `verify.launch_test.survived` is true or you
manually opened the clone and confirmed it stays alive.

# Fake az, kubectl and helm

These are not the real tools. Each is a two-line bash script that hands its
arguments to `../fake.sh`, so the Go tests in `deploy/azure` can run the deploy
scripts without touching a cloud account or a cluster.

`runScript` (in `azure_test.go`) puts this directory first on `PATH`, then runs
a script under bash. Every `az`, `kubectl` or `helm` call the script makes lands
here instead. `fake.sh` then:

- appends `<tool> <args>` to `$FAKE_LOG`, in call order, so a test can assert
  exactly which commands ran (piped stdin is saved beside the log);
- exits 3 when the call matches the glob `$FAKE_FAIL`, to test error paths,
  printing `$FAKE_FAIL_MSG` to stderr when that is set;
- otherwise prints a canned answer from `$FAKE_FIXTURES/<args>.json`, if one
  matches.

The fake `az` also rejects a global flag placed before the command group, as the
real one does, so a wrong argument order cannot pass the tests.

Nothing here is a binary, and nothing here is used outside the tests.

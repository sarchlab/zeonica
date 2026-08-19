# FFT integration test

The FFT test depends on the generated program in the `test/Zeonica_Testbench`
submodule, so it is intentionally excluded from the default unit-test suite.

Initialize the testbench and run it explicitly with:

```bash
git submodule update --init test/Zeonica_Testbench
go test -tags=integration ./test/testbench/fft
```

Unlike the previous optional test, the integration test fails when its fixture
is missing instead of silently reporting a skipped test.

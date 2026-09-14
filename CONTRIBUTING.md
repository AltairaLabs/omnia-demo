# Contributing

Changes to this repository are made through pull requests. A maintainer must
approve a pull request before it can merge, and the required CI checks must be
green.

Keep this repository public-safe:

- use synthetic data and credentials supplied through values or Kubernetes
  Secrets;
- do not copy private Omnia platform code, unreleased APIs, customer material,
  internal endpoints, or private container images;
- keep platform changes in [Omnia](https://github.com/AltairaLabs/Omnia);
- make each demo independently disableable through Helm values;
- document Enterprise-only prerequisites explicitly.

Before opening a pull request, run the checks described in `.github/workflows/ci.yml`.


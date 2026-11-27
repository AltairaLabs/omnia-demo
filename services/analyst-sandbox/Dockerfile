# The grid analyst's sandbox: Omnia's codegen sandbox (an MCP server with
# file, shell and test tools) plus Python with pandas and matplotlib, so the
# analyst can answer a question by writing and running an analysis, the way a
# person would. Pinned to the same base as Omnia's promptarena sandbox.
FROM ghcr.io/altairalabs/codegen-sandbox:v0.1.8@sha256:40924fb2968f635805bcd663e8376b51ad78b2968b4fbd6bfaebcba258ec2eaa

USER root
# Alpine's own builds: no compiler, no pip, nothing fetched at run time (the
# sandbox has no network).
RUN apk upgrade --no-cache \
    && apk add --no-cache python3 py3-pandas py3-matplotlib \
    && python3 -c "import pandas, matplotlib; matplotlib.use('Agg')"
# matplotlib writes its font cache under $HOME, which the sandbox class sets to
# a scratch volume; Agg needs no display.
ENV MPLBACKEND=Agg
USER sandbox

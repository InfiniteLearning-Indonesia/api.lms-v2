# Build context is ONLY the generated release directory, never the repository.
# No shell, package manager, credentials, database client or CA bundle included.
# This artifact is loopback bootstrap only; no shared-network mode is enabled.
FROM scratch
COPY --chown=65532:65532 --chmod=0555 api /api
USER 65532:65532
ENTRYPOINT ["/api"]

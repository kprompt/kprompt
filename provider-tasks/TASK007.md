# TASK007 — Refresh stale default models

## Goal
Refresh stale default models used by the provider to ensure hygiene and up-to-date baseline configurations.

## Background
Over time, default models embedded in the provider can become outdated or stale, leading to potential issues or inconsistencies. This task aims to refresh those models to the latest stable versions, improving reliability and maintainability.

## Acceptance Criteria
- Default models are updated to the latest stable versions.
- The update is implemented in the primary repository.
- Tests validate that the refreshed models load correctly and behave as expected.
- Documentation or CHANGELOG entries reflect the update if user-visible.
- The PR references this task and GitHub issue #219.

## Implementation Notes
- Identify the current default models and their sources.
- Replace or update the models with the latest stable versions.
- Add or update tests to cover loading and usage of the refreshed models.
- Update documentation or changelog if the update affects user experience.

## Testing
- Run existing tests to ensure no regressions.
- Add tests specifically verifying the refreshed models.
- Validate that the provider starts and operates correctly with the new models.

## References
- GitHub issue #219

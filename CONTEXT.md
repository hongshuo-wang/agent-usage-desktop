# Agent Usage

This context describes the local activity that Agent Usage observes across supported AI coding agents.

## Language

**Usage source**:
An AI coding agent whose local records contribute activity to Agent Usage.
_Avoid_: Provider, integration

**Usage event**:
One observed model interaction with non-overlapping token components and an event time.
_Avoid_: Request log, transaction

**Session retrospective**:
A searchable reconstruction of a coding session's recorded prompts, responses, tool activity, and errors.
_Avoid_: Session replay, transcript

**Observed throughput**:
The API-call and token rate visible in local source records during a time window; it does not describe provider quota or remaining capacity.
_Avoid_: Rate-limit usage, quota utilization

**Model pricing catalog**:
The active collection of model token rates used to estimate usage costs. It may originate from the bundled catalog, a remote refresh, or a user import.
_Avoid_: Price file, model price list

**Unpriced usage**:
Usage for which no matching model rate was available at the event time; it is excluded from estimated cost until a valid price can be assigned.
_Avoid_: Free usage, zero-cost usage

# Jira Issue Collection Strategy

Author: Timothy Williams (Not AI)

## Problem #1

There are three ways I know of that teams define their ownership:
- Issues with a list of specific Project + Component combinations
- Issues with the "Team" field set to a particular value on issues in a project
- Issues that are pulled into their Team's sprint in Jira

This last one is an issue. We need to be able to map each team to a predictable list of sprint IDs. Sprints are created
on an unpredictable cadence (at most every 3 weeks) and attached to a board. There is no mapping in Jira of Board ->
Team. So our mapping needs to support specifying a list of boards for a team, then the collection needs to be able to
gather the sprints from that board, then use the sprint as a filter in the JQL.

## Option exploration

### Ignore sprints

If we ignore sprints entirely, its similar to what we do today. We can treat the 3-week cadence from a reference point
as completely arbitrary. This means that teams following kanban or _nothing_ still have issues show in the cadence.

However, this breaks the ability to define some teams, as teams like Coffee and Focaccia define ownership as "If its in
my sprint, I own it". Those teams share components, so which team owns the issue doesn't get decided until a team add
its to their sprint. These teams are outliers, every other team has some consistent way to define ownership, whether it
be component or team field value. Perhaps we treat these as poor examples and use this method?

To fix Coffee/Focaccia, we could implement an automation in the OCM project that would set the Team field value, or even
just set a label that we could use to identify the issues. We can't reliably use the sprint ID because sprints are
created every 3 weeks, it changes and will be an unpredictable value.

### Add support for `sprint-board` method

In the resource mapping, we could add a `sprint-board` method. This would accept a list of board IDs. Boards define
sprints, including an active sprint. There are even JQL helpers to help us get active/closed/etc sprints. We could use
this new method to support teams like coffee/foccacia in being able to gather only the issues they've specifically
pulled into a sprint associated with their board.

To support this though, we'd need to make it obvious in the scorecard that the sprint is assumed to be the 3 week
cadence based on reference date. If a team defines sprints on a different cadence, they may be misled.

## Solution Decision

I think we should do a mix of both, where we add the `sprint-board` method and make it clear that the 3-week-cadence is
not necessarily a sprint, but just an arbitrary cadence.

## Problem #2

Issue collection relies on the `updated_at` field value to determine which 3-week cadence an issue belongs in. Issues
can be updated long after completion, or long before the work is ever started.

### Solution

We should score on two sets of issues:
- issues _moved to closed status_ within each cadence
- issues currently in an in-progress-like status (configurable list, example: In Progress, Code Review, Review)

This would essentially divide every scorecard into two sections, one focused on the past (closed issues), and one
focused on the current reality (in-progress issues).

In this way, we cover all issues worked or being worked accurately.

## Resources

I asked the jira administration agent how I can gather issues for a given sprint if I know the name. Below is the
response:
```
  Quick Answer

  For "ROSA Focaccia Sprint 7", you can use:

  JQL syntax:
  sprint = "ROSA Focaccia Sprint 7"

  Jira CLI:
  jira issue list --jql 'sprint = "ROSA Focaccia Sprint 7"'

  REST API (what your sankey-scorecard already uses):
  curl -X GET \
    -H "Authorization: Bearer ${JIRA_API_TOKEN}" \
    "https://issues.redhat.com/rest/api/2/search?jql=sprint%3D%22ROSA%20Focaccia%20Sprint%207%22&maxResults=100"

  Key Points

  1. Sprint name matching is exact and case-sensitive - use quotes for names with spaces
  2. Sprint IDs are more reliable than names (which can change)
  3. Useful JQL functions for dynamic queries:
    - sprint in openSprints() - active sprints
    - sprint in futureSprints() - upcoming sprints
    - sprint in closedSprints() - completed sprints
  4. Find sprint IDs via:
    - jira sprint list --board "ROSA Focaccia"
    - Jira UI URL: ?rapidView={boardId}&sprint={sprintId}
    - Agile API: /rest/agile/1.0/board/{boardId}/sprint

  The agent also noted that your existing pkg/reconciler code already uses the /rest/api/2/search endpoint, so you can easily
  integrate sprint filtering by adding sprint criteria to your JQL queries.
```

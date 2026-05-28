export type LinearWorkflowStateType =
  | "backlog"
  | "unstarted"
  | "started"
  | "completed"
  | "canceled"
  | string;

export interface LinearWorkflowState {
  id: string;
  name: string;
  type: LinearWorkflowStateType;
}

export interface LinearIssueReference {
  id: string;
  identifier?: string;
  title?: string;
  state?: LinearWorkflowState;
}

export interface LinearIssue {
  id: string;
  identifier?: string;
  title: string;
  state: LinearWorkflowState;
  parentId?: string | null;
  parent?: LinearIssueReference | null;
  blockedBy?: LinearIssueReference[];
  archivedAt?: string | null;
}

export interface LinearIssueClient {
  getIssue(issueId: string): Promise<LinearIssue>;
  listIssueStatuses(issue: LinearIssue): Promise<LinearWorkflowState[]>;
  updateIssueState(issueId: string, stateId: string): Promise<LinearIssue>;
}

export interface LinearIssueTransition {
  issueId: string;
  from: LinearWorkflowState;
  to: LinearWorkflowState;
  changed: boolean;
}

export interface StartLinearIssueResult {
  issue: LinearIssueTransition;
  parent: LinearIssueTransition | null;
}

export interface StartLinearIssueOptions {
  allowProtectedParent?: boolean;
}

export class LinearIssueStartBlockedError extends Error {
  readonly blockers: LinearIssueReference[];

  constructor(issue: LinearIssue, blockers: LinearIssueReference[]) {
    const blockerList = blockers
      .map((blocker) => formatIssueReference(blocker))
      .join(", ");
    super(`Cannot start ${formatIssueReference(issue)}. Blocked by: ${blockerList}`);
    this.name = "LinearIssueStartBlockedError";
    this.blockers = blockers;
  }
}

export class LinearProtectedParentError extends Error {
  readonly parent: LinearIssue;

  constructor(parent: LinearIssue) {
    super(`Cannot start child issue because parent ${formatIssueReference(parent)} is ${parent.state.name}`);
    this.name = "LinearProtectedParentError";
    this.parent = parent;
  }
}

export class LinearParentPromotionError extends Error {
  readonly parentId: string;
  readonly childStarted: boolean;

  constructor(parentId: string, cause: unknown, childStarted: boolean) {
    const causeMessage = cause instanceof Error ? cause.message : String(cause);
    super(`Started child issue, but failed to promote parent ${parentId}: ${causeMessage}`);
    this.name = "LinearParentPromotionError";
    this.parentId = parentId;
    this.childStarted = childStarted;
  }
}

export async function startLinearIssue(
  client: LinearIssueClient,
  issueId: string,
  options: StartLinearIssueOptions = {},
): Promise<StartLinearIssueResult> {
  const issue = await client.getIssue(issueId);
  assertIssueCanStart(issue);

  const blockers = await resolveOpenBlockers(client, issue);
  if (blockers.length > 0) {
    throw new LinearIssueStartBlockedError(issue, blockers);
  }

  const parentId = getParentId(issue);
  const parent = parentId ? await client.getIssue(parentId) : null;
  if (parent && isProtectedParent(parent) && !options.allowProtectedParent) {
    throw new LinearProtectedParentError(parent);
  }

  const startedState = await getStartedState(client, issue);
  const issueTransition = await transitionIssueToState(client, issue, startedState);

  let parentTransition: LinearIssueTransition | null = null;
  if (parent) {
    parentTransition = await promoteParentIfNeeded(
      client,
      parent,
      issueTransition.changed,
    );
  }

  return {
    issue: issueTransition,
    parent: parentTransition,
  };
}

async function resolveOpenBlockers(
  client: LinearIssueClient,
  issue: LinearIssue,
): Promise<LinearIssueReference[]> {
  const blockers = issue.blockedBy ?? [];
  const openBlockers: LinearIssueReference[] = [];

  for (const blocker of blockers) {
    const resolved = blocker.state ? blocker : await client.getIssue(blocker.id);
    if (!resolved.state || !isCompletedState(resolved.state)) {
      openBlockers.push(resolved);
    }
  }

  return openBlockers;
}

async function promoteParentIfNeeded(
  client: LinearIssueClient,
  parent: LinearIssue,
  childStarted: boolean,
): Promise<LinearIssueTransition> {
  if (!shouldPromoteParent(parent)) {
    return {
      issueId: parent.id,
      from: parent.state,
      to: parent.state,
      changed: false,
    };
  }

  const startedState = await getStartedState(client, parent);
  try {
    return await transitionIssueToState(client, parent, startedState);
  } catch (err) {
    throw new LinearParentPromotionError(formatIssueReference(parent), err, childStarted);
  }
}

async function transitionIssueToState(
  client: LinearIssueClient,
  issue: LinearIssue,
  targetState: LinearWorkflowState,
): Promise<LinearIssueTransition> {
  if (issue.state.id === targetState.id) {
    return {
      issueId: issue.id,
      from: issue.state,
      to: targetState,
      changed: false,
    };
  }

  await client.updateIssueState(issue.id, targetState.id);
  return {
    issueId: issue.id,
    from: issue.state,
    to: targetState,
    changed: true,
  };
}

async function getStartedState(
  client: LinearIssueClient,
  issue: LinearIssue,
): Promise<LinearWorkflowState> {
  const states = await client.listIssueStatuses(issue);
  const startedState = states.find((state) => normalizeStateType(state.type) === "started");
  if (!startedState) {
    throw new Error(`No started-type Linear state found for ${formatIssueReference(issue)}`);
  }
  return startedState;
}

function assertIssueCanStart(issue: LinearIssue): void {
  if (issue.archivedAt) {
    throw new Error(`Cannot start archived issue ${formatIssueReference(issue)}`);
  }
  if (isProtectedState(issue.state)) {
    throw new Error(`Cannot start ${formatIssueReference(issue)} from protected state ${issue.state.name}`);
  }
}

function shouldPromoteParent(parent: LinearIssue): boolean {
  const stateType = normalizeStateType(parent.state.type);
  return stateType === "backlog" || stateType === "unstarted";
}

function isProtectedParent(parent: LinearIssue): boolean {
  return Boolean(parent.archivedAt) || isProtectedState(parent.state);
}

function isProtectedState(state: LinearWorkflowState): boolean {
  const stateType = normalizeStateType(state.type);
  return stateType === "completed" || stateType === "canceled";
}

function isCompletedState(state: LinearWorkflowState): boolean {
  return normalizeStateType(state.type) === "completed";
}

function normalizeStateType(type: LinearWorkflowStateType): string {
  return type.toLowerCase().replace(/[-\s]/g, "_");
}

function getParentId(issue: LinearIssue): string | null {
  return issue.parentId ?? issue.parent?.id ?? null;
}

function formatIssueReference(issue: LinearIssueReference): string {
  return issue.identifier ?? issue.id;
}

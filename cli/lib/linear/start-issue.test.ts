import { describe, expect, it } from "vitest";

import {
  LinearIssueStartBlockedError,
  LinearParentPromotionError,
  LinearProtectedParentError,
  startLinearIssue,
  type LinearIssue,
  type LinearIssueClient,
  type LinearWorkflowState,
} from "./start-issue.js";

const backlog = state("backlog", "Backlog", "backlog");
const todo = state("todo", "Todo", "unstarted");
const started = state("started", "In Progress", "started");
const completed = state("completed", "Done", "completed");
const canceled = state("canceled", "Canceled", "canceled");

function state(id: string, name: string, type: string): LinearWorkflowState {
  return { id, name, type };
}

function issue(overrides: Partial<LinearIssue>): LinearIssue {
  return {
    id: "child-id",
    identifier: "ENG-123",
    title: "Child issue",
    state: todo,
    ...overrides,
  };
}

function fakeClient(
  issues: Record<string, LinearIssue>,
  options: { failUpdateFor?: string } = {},
): LinearIssueClient & { updates: Array<{ issueId: string; stateId: string }> } {
  return {
    updates: [],
    async getIssue(issueId: string): Promise<LinearIssue> {
      const found = issues[issueId];
      if (!found) throw new Error(`missing issue ${issueId}`);
      return found;
    },
    async listIssueStatuses(): Promise<LinearWorkflowState[]> {
      return [backlog, todo, started, completed, canceled];
    },
    async updateIssueState(issueId: string, stateId: string): Promise<LinearIssue> {
      if (options.failUpdateFor === issueId) {
        throw new Error(`update failed for ${issueId}`);
      }
      this.updates.push({ issueId, stateId });
      return {
        ...issues[issueId],
        state: stateId === started.id ? started : issues[issueId].state,
      };
    },
  };
}

describe("startLinearIssue", () => {
  it("starts a child issue and promotes an unstarted parent", async () => {
    const child = issue({ id: "child-id", parentId: "parent-id" });
    const parent = issue({
      id: "parent-id",
      identifier: "ENG-100",
      title: "Parent rollup",
      state: todo,
    });
    const client = fakeClient({ "child-id": child, "parent-id": parent });

    const result = await startLinearIssue(client, "child-id");

    expect(client.updates).toEqual([
      { issueId: "child-id", stateId: "started" },
      { issueId: "parent-id", stateId: "started" },
    ]);
    expect(result.issue.changed).toBe(true);
    expect(result.parent?.changed).toBe(true);
  });

  it("does not update the parent when it is already active", async () => {
    const child = issue({ id: "child-id", parentId: "parent-id" });
    const parent = issue({
      id: "parent-id",
      identifier: "ENG-100",
      title: "Parent rollup",
      state: started,
    });
    const client = fakeClient({ "child-id": child, "parent-id": parent });

    const result = await startLinearIssue(client, "child-id");

    expect(client.updates).toEqual([{ issueId: "child-id", stateId: "started" }]);
    expect(result.parent?.changed).toBe(false);
  });

  it("does not move any issue when an open blocker remains", async () => {
    const blocker = issue({
      id: "blocker-id",
      identifier: "ENG-122",
      title: "Blocker",
      state: started,
    });
    const child = issue({
      id: "child-id",
      parentId: "parent-id",
      blockedBy: [{ id: "blocker-id" }],
    });
    const parent = issue({ id: "parent-id", identifier: "ENG-100", title: "Parent" });
    const client = fakeClient({ "child-id": child, "parent-id": parent, "blocker-id": blocker });

    await expect(startLinearIssue(client, "child-id")).rejects.toBeInstanceOf(
      LinearIssueStartBlockedError,
    );
    expect(client.updates).toEqual([]);
  });

  it("does not move any issue when the parent is completed", async () => {
    const child = issue({ id: "child-id", parentId: "parent-id" });
    const parent = issue({
      id: "parent-id",
      identifier: "ENG-100",
      title: "Parent rollup",
      state: completed,
    });
    const client = fakeClient({ "child-id": child, "parent-id": parent });

    await expect(startLinearIssue(client, "child-id")).rejects.toBeInstanceOf(
      LinearProtectedParentError,
    );
    expect(client.updates).toEqual([]);
  });

  it("reports reconciliation failure when child start succeeds but parent promotion fails", async () => {
    const child = issue({ id: "child-id", parentId: "parent-id" });
    const parent = issue({
      id: "parent-id",
      identifier: "ENG-100",
      title: "Parent rollup",
      state: backlog,
    });
    const client = fakeClient(
      { "child-id": child, "parent-id": parent },
      { failUpdateFor: "parent-id" },
    );

    await expect(startLinearIssue(client, "child-id")).rejects.toBeInstanceOf(
      LinearParentPromotionError,
    );
    expect(client.updates).toEqual([{ issueId: "child-id", stateId: "started" }]);
  });

  it("starts a top-level issue without parent promotion", async () => {
    const topLevel = issue({ id: "issue-id", parentId: null });
    const client = fakeClient({ "issue-id": topLevel });

    const result = await startLinearIssue(client, "issue-id");

    expect(client.updates).toEqual([{ issueId: "issue-id", stateId: "started" }]);
    expect(result.parent).toBeNull();
  });
});

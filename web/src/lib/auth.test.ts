import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cancelRun, getRun, listRuns } from "@/lib/auth";

const fetchMock = vi.fn();

describe("runs API client", () => {
  beforeEach(() => {
    fetchMock.mockReset();
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("lists runs with cursor pagination and exposed next cursor", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify([
          {
            id: "run-1",
            job_type: "sync",
            trigger: "api",
            status: "pending",
            started_at: null,
            finished_at: null,
            meta_json: {},
          },
        ]),
        {
          status: 200,
          headers: {
            "Content-Type": "application/json",
            "X-Next-Cursor": "cursor-2",
          },
        },
      ),
    );

    const result = await listRuns("token", {
      accountId: "acc-1",
      jobType: "sync",
      limit: 25,
      cursor: "cursor-1",
    });

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/runs?account_id=acc-1&job_type=sync&limit=25&cursor=cursor-1",
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: "Bearer token",
        }),
      }),
    );
    expect(result).toEqual({
      runs: [
        expect.objectContaining({
          id: "run-1",
          started_at: null,
          finished_at: null,
        }),
      ],
      nextCursor: "cursor-2",
    });
  });

  it("rejects positive offset pagination", async () => {
    await expect(listRuns("token", { offset: 1 })).rejects.toThrow(
      "offset pagination not supported; use cursor",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("loads a single run with nullable timestamps", async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          id: "run-1",
          job_type: "sync",
          trigger: "api",
          status: "running",
          started_at: null,
          finished_at: null,
          meta_json: { processed_messages: 5 },
        }),
        {
          status: 200,
          headers: {
            "Content-Type": "application/json",
          },
        },
      ),
    );

    const run = await getRun("token", "run-1");

    expect(run.started_at).toBeNull();
    expect(run.finished_at).toBeNull();
  });

  it("posts run cancellation without requiring a JSON body", async () => {
    fetchMock.mockResolvedValue(new Response(null, { status: 204 }));

    await expect(cancelRun("token", "run-1")).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(
      "http://localhost:8080/api/runs/run-1/cancel",
      expect.objectContaining({
        method: "POST",
        headers: expect.objectContaining({
          Authorization: "Bearer token",
        }),
      }),
    );
  });
});

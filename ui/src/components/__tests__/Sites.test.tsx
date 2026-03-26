import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Sites } from "../Sites";

vi.mock("../../api_sites", () => ({
  listSites: vi.fn(),
  createSite: vi.fn(),
  getSite: vi.fn(),
  updateSite: vi.fn(),
  deleteSite: vi.fn(),
  listDeploys: vi.fn(),
  promoteDeploy: vi.fn(),
  rollbackDeploy: vi.fn(),
}));

import * as api from "../../api_sites";

const mockSite = {
  id: "site-1",
  name: "Marketing",
  slug: "marketing",
  spaMode: true,
  createdAt: "2026-03-10T10:00:00Z",
  updatedAt: "2026-03-10T10:00:00Z",
  liveDeployId: "dep-1",
};

const mockSitesResult = {
  sites: [mockSite],
  totalCount: 1,
  page: 1,
  perPage: 20,
};

const mockSecondSite = {
  ...mockSite,
  id: "site-2",
  name: "Docs",
  slug: "docs",
  liveDeployId: undefined,
};

const mockDeploysResult = {
  deploys: [
    {
      id: "dep-1",
      siteId: "site-1",
      status: "live",
      fileCount: 12,
      totalBytes: 1024,
      createdAt: "2026-03-10T10:05:00Z",
      updatedAt: "2026-03-10T10:05:00Z",
    },
    {
      id: "dep-2",
      siteId: "site-1",
      status: "superseded",
      fileCount: 8,
      totalBytes: 512,
      createdAt: "2026-03-09T10:05:00Z",
      updatedAt: "2026-03-09T10:05:00Z",
    },
  ],
  totalCount: 2,
  page: 1,
  perPage: 20,
};

const mockSecondDeployPage = {
  deploys: [
    {
      id: "dep-3",
      siteId: "site-1",
      status: "uploading",
      fileCount: 3,
      totalBytes: 256,
      createdAt: "2026-03-08T10:05:00Z",
      updatedAt: "2026-03-08T10:05:00Z",
    },
  ],
  totalCount: 2,
  page: 2,
  perPage: 1,
};

beforeEach(() => {
  vi.clearAllMocks();
  (api.listSites as ReturnType<typeof vi.fn>).mockResolvedValue(mockSitesResult);
  (api.createSite as ReturnType<typeof vi.fn>).mockResolvedValue(mockSite);
  (api.getSite as ReturnType<typeof vi.fn>).mockResolvedValue(mockSite);
  (api.updateSite as ReturnType<typeof vi.fn>).mockResolvedValue(mockSite);
  (api.deleteSite as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.listDeploys as ReturnType<typeof vi.fn>).mockResolvedValue(mockDeploysResult);
  (api.promoteDeploy as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.rollbackDeploy as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
});

async function renderSites() {
  renderWithProviders(<Sites />);

  await waitFor(() => {
    expect(screen.getByText("Marketing")).toBeInTheDocument();
  });
}

async function openMarketingDetail() {
  fireEvent.click(screen.getByRole("button", { name: /view marketing/i }));

  await waitFor(() => {
    expect(screen.getByDisplayValue("Marketing")).toBeInTheDocument();
  });
}

describe("Sites", () => {
  it("renders sites list with name/slug/status columns", async () => {
    await renderSites();

    expect(screen.getByText("Name")).toBeInTheDocument();
    expect(screen.getByText("Slug")).toBeInTheDocument();
    expect(screen.getByText("Live Deploy")).toBeInTheDocument();
    expect(screen.getByText("Created"))
      .toBeInTheDocument();
    expect(screen.getByText("marketing")).toBeInTheDocument();
  });

  it("create form validates required fields", async () => {
    await renderSites();

    fireEvent.click(screen.getByRole("button", { name: /add site/i }));

    const createButton = screen.getByRole("button", { name: /^create$/i });
    expect(createButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Docs" } });
    expect(createButton).toBeDisabled();

    fireEvent.change(screen.getByLabelText("Slug"), { target: { value: "docs" } });
    expect(createButton).not.toBeDisabled();
  });

  it("create calls createSite with expected payload", async () => {
    await renderSites();

    fireEvent.click(screen.getByRole("button", { name: /add site/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Docs" } });
    fireEvent.change(screen.getByLabelText("Slug"), { target: { value: "docs" } });
    fireEvent.click(screen.getByLabelText("SPA mode"));
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(api.createSite).toHaveBeenCalledWith({
        name: "Docs",
        slug: "docs",
        spaMode: true,
      });
    });
  });

  it("keeps the sites list visible when create fails", async () => {
    (api.createSite as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Slug taken"),
    );

    await renderSites();

    fireEvent.click(screen.getByRole("button", { name: /add site/i }));
    fireEvent.change(screen.getByLabelText("Name"), { target: { value: "Docs" } });
    fireEvent.change(screen.getByLabelText("Slug"), { target: { value: "docs" } });
    fireEvent.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(screen.getByText("Slug taken")).toBeInTheDocument();
    });

    expect(screen.getByText("Marketing")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /new site/i })).toBeInTheDocument();
  });

  it("delete fires ConfirmDialog and calls deleteSite", async () => {
    await renderSites();

    fireEvent.click(screen.getByLabelText("Delete Marketing"));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /delete site/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));

    await waitFor(() => {
      expect(api.deleteSite).toHaveBeenCalledWith("site-1");
    });
  });

  it("keeps the sites list visible when delete fails", async () => {
    (api.deleteSite as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Delete failed"),
    );

    await renderSites();

    fireEvent.click(screen.getByLabelText("Delete Marketing"));
    fireEvent.click(screen.getByRole("button", { name: /^delete$/i }));

    await waitFor(() => {
      expect(screen.getByText("Delete failed")).toBeInTheDocument();
    });

    expect(screen.getByRole("heading", { name: /^sites$/i })).toBeInTheDocument();
    expect(screen.getByText("Marketing")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /delete site/i })).toBeInTheDocument();
  });

  it("renders error state on list fetch failure", async () => {
    (api.listSites as ReturnType<typeof vi.fn>).mockRejectedValue(new Error("List failed"));

    renderWithProviders(<Sites />);

    await waitFor(() => {
      expect(screen.getByText("List failed")).toBeInTheDocument();
    });
  });

  it("detail view renders site settings and deploy table", async () => {
    await renderSites();
    await openMarketingDetail();

    expect(screen.getByRole("heading", { name: /site settings/i })).toBeInTheDocument();
    expect(screen.getByDisplayValue("Marketing")).toBeInTheDocument();
    expect(screen.getByDisplayValue("marketing")).toBeInTheDocument();
    expect(screen.getByText("Deploy History")).toBeInTheDocument();
    expect(screen.getByText("Status")).toBeInTheDocument();
    expect(screen.getByText("File Count")).toBeInTheDocument();
    expect(screen.getByText("Created"))
      .toBeInTheDocument();
    expect(screen.getByText("live")).toBeInTheDocument();
    expect(screen.getByText("superseded")).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
  });

  it("loads the selected sites page when pagination advances", async () => {
    (api.listSites as ReturnType<typeof vi.fn>).mockImplementation(
      async (params?: { page?: number }) => {
        if (params?.page === 2) {
          return {
            sites: [mockSecondSite],
            totalCount: 2,
            page: 2,
            perPage: 1,
          };
        }
        return {
          sites: [mockSite],
          totalCount: 2,
          page: 1,
          perPage: 1,
        };
      },
    );

    renderWithProviders(<Sites />);

    await waitFor(() => {
      expect(screen.getByText("Marketing")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));

    await waitFor(() => {
      expect(api.listSites).toHaveBeenLastCalledWith({ page: 2 });
    });

    expect(await screen.findByText("Docs")).toBeInTheDocument();
  });

  it("promote action calls promoteDeploy with site and deploy ids", async () => {
    renderWithProviders(<Sites />);

    await waitFor(() => {
      expect(screen.getByText("Marketing")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /view marketing/i }));

    await waitFor(() => {
      expect(screen.getByText("superseded")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /promote dep-2/i }));

    await waitFor(() => {
      expect(api.promoteDeploy).toHaveBeenCalledWith("site-1", "dep-2");
    });
  });

  it("keeps detail content visible when a deploy action fails", async () => {
    (api.promoteDeploy as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Promote failed"),
    );

    await renderSites();

    await openMarketingDetail();
    await waitFor(() => {
      expect(screen.getByText("superseded")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /promote dep-2/i }));

    await waitFor(() => {
      expect(screen.getByText("Promote failed")).toBeInTheDocument();
    });

    expect(screen.getByRole("heading", { name: /site settings/i })).toBeInTheDocument();
    expect(screen.getByText("Deploy History")).toBeInTheDocument();
  });

  it("rollback action calls rollbackDeploy", async () => {
    await renderSites();
    await openMarketingDetail();

    fireEvent.click(screen.getByRole("button", { name: /rollback site/i }));

    await waitFor(() => {
      expect(api.rollbackDeploy).toHaveBeenCalledWith("site-1");
    });
  });

  it("loads the selected deploy page when pagination advances", async () => {
    (api.listDeploys as ReturnType<typeof vi.fn>).mockImplementation(
      async (_siteId: string, params?: { page?: number }) => {
        if (params?.page === 2) {
          return mockSecondDeployPage;
        }
        return {
          deploys: [mockDeploysResult.deploys[0]],
          totalCount: 2,
          page: 1,
          perPage: 1,
        };
      },
    );

    renderWithProviders(<Sites />);

    await waitFor(() => {
      expect(screen.getByText("Marketing")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /view marketing/i }));

    await waitFor(() => {
      expect(screen.getByText("live")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /next page/i }));

    await waitFor(() => {
      expect(api.listDeploys).toHaveBeenLastCalledWith("site-1", { page: 2 });
    });

    expect(await screen.findByText("uploading")).toBeInTheDocument();
  });

  it("keeps showing detail loading state until site settings hydrate", async () => {
    let resolveSite: ((site: typeof mockSite) => void) | undefined;
    const pendingSite = new Promise<typeof mockSite>((resolve) => {
      resolveSite = resolve;
    });
    (api.getSite as ReturnType<typeof vi.fn>).mockReturnValueOnce(pendingSite);

    await renderSites();

    fireEvent.click(screen.getByRole("button", { name: /view marketing/i }));

    await waitFor(() => {
      expect(screen.getByText("Loading site...")).toBeInTheDocument();
    });
    expect(screen.queryByDisplayValue("Marketing")).not.toBeInTheDocument();

    resolveSite?.(mockSite);

    await waitFor(() => {
      expect(screen.getByDisplayValue("Marketing")).toBeInTheDocument();
    });
  });

  it("save settings calls updateSite with name and spaMode", async () => {
    await renderSites();
    await openMarketingDetail();

    fireEvent.change(screen.getByDisplayValue("Marketing"), {
      target: { value: "new name" },
    });

    fireEvent.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(api.updateSite).toHaveBeenCalledWith("site-1", {
        name: "new name",
        spaMode: true,
      });
    });
  });

  it("keeps detail content visible when saving settings fails", async () => {
    (api.updateSite as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error("Save failed"),
    );

    await renderSites();
    await openMarketingDetail();

    fireEvent.change(screen.getByDisplayValue("Marketing"), {
      target: { value: "Broken rename" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(screen.getByText("Save failed")).toBeInTheDocument();
    });

    expect(screen.getByRole("heading", { name: /site settings/i })).toBeInTheDocument();
    expect(screen.getByText("Deploy History")).toBeInTheDocument();
    expect(screen.getByDisplayValue("Broken rename")).toBeInTheDocument();
  });

  it("back to list refreshes renamed site after save", async () => {
    const renamedSite = {
      ...mockSite,
      name: "Marketing Updated",
    };
    let listCallCount = 0;
    let getSiteCallCount = 0;

    (api.listSites as ReturnType<typeof vi.fn>).mockImplementation(async () => {
      listCallCount += 1;
      return {
        ...mockSitesResult,
        sites: [listCallCount >= 2 ? renamedSite : mockSite],
      };
    });
    (api.getSite as ReturnType<typeof vi.fn>).mockImplementation(async () => {
      getSiteCallCount += 1;
      return getSiteCallCount >= 2 ? renamedSite : mockSite;
    });
    (api.updateSite as ReturnType<typeof vi.fn>).mockResolvedValue(renamedSite);

    renderWithProviders(<Sites />);

    await waitFor(() => {
      expect(screen.getByText("Marketing")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /view marketing/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /site settings/i })).toBeInTheDocument();
    });

    fireEvent.change(screen.getByDisplayValue("Marketing"), {
      target: { value: renamedSite.name },
    });
    fireEvent.click(screen.getByRole("button", { name: /save settings/i }));

    await waitFor(() => {
      expect(api.updateSite).toHaveBeenCalledWith("site-1", {
        name: renamedSite.name,
        spaMode: true,
      });
    });

    fireEvent.click(screen.getByRole("button", { name: /back to sites/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /^sites$/i })).toBeInTheDocument();
    });

    expect(await screen.findByText(renamedSite.name)).toBeInTheDocument();
    expect(screen.getByLabelText(`Delete ${renamedSite.name}`)).toBeInTheDocument();
    expect(screen.queryByText("Marketing")).not.toBeInTheDocument();
  });

  it("back button returns from detail to list", async () => {
    renderWithProviders(<Sites />);

    await waitFor(() => {
      expect(screen.getByText("Marketing")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /view marketing/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /site settings/i })).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /back to sites/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /^sites$/i })).toBeInTheDocument();
    });

    expect(screen.getByText("Marketing")).toBeInTheDocument();
  });
});

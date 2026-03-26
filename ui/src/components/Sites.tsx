import { useEffect, useRef, useState } from "react";
import {
  createSite,
  deleteSite,
  getSite,
  listDeploys,
  listSites,
  promoteDeploy,
  rollbackDeploy,
  updateSite,
} from "../api_sites";
import type { Deploy, Site } from "../types/sites";
import { useAdminResource } from "../hooks/useAdminResource";
import { AdminTable, type Column } from "./shared/AdminTable";
import { ConfirmDialog } from "./shared/ConfirmDialog";
import { StatusBadge } from "./shared/StatusBadge";
import { formatBytes, formatDate } from "./shared/format";

const SITE_STATUS_VARIANTS: Record<string, "success" | "default"> = {
  live: "success",
  none: "default",
};

const DEPLOY_STATUS_VARIANTS: Record<
  string,
  "success" | "info" | "default" | "error"
> = {
  live: "success",
  uploading: "info",
  superseded: "default",
  failed: "error",
};

type SitesViewState = { kind: "list" } | { kind: "detail"; siteId: string };

function getTotalPages(totalCount?: number, perPage?: number): number {
  if (totalCount == null || perPage == null) {
    return 1;
  }
  return Math.max(1, Math.ceil(totalCount / Math.max(perPage, 1)));
}

function useRefreshOnPageChange(
  page: number,
  refresh: () => Promise<void>,
) {
  const hasMountedRef = useRef(false);

  useEffect(() => {
    if (!hasMountedRef.current) {
      hasMountedRef.current = true;
      return;
    }
    void refresh();
  }, [page, refresh]);
}

function useClampedPage(
  page: number,
  setPage: (page: number) => void,
  totalCount?: number,
  perPage?: number,
) {
  const totalPages = getTotalPages(totalCount, perPage);

  useEffect(() => {
    if ((totalCount ?? 0) > 0 && page > totalPages) {
      setPage(totalPages);
    }
  }, [page, setPage, totalCount, totalPages]);

  return totalPages;
}

function createDeployColumns(
  onPromoteDeploy: (deployId: string) => void,
  actionLoading: boolean,
): Column<Deploy>[] {
  return [
    {
      key: "status",
      header: "Status",
      render: (row) => (
        <StatusBadge status={row.status} variantMap={DEPLOY_STATUS_VARIANTS} />
      ),
    },
    { key: "fileCount", header: "File Count" },
    {
      key: "totalBytes",
      header: "Total Bytes",
      render: (row) => formatBytes(row.totalBytes),
    },
    {
      key: "createdAt",
      header: "Created",
      render: (row) => formatDate(row.createdAt),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <div className="flex gap-2">
          {(row.status === "uploading" || row.status === "superseded") && (
            <button
              aria-label={`Promote ${row.id}`}
              onClick={() => onPromoteDeploy(row.id)}
              disabled={actionLoading}
              className="text-xs text-blue-500 hover:text-blue-600 disabled:opacity-50"
            >
              Promote
            </button>
          )}
        </div>
      ),
    },
  ];
}

function createSiteColumns(
  onViewSite: (siteId: string) => void,
  onDeleteSite: (site: Site) => void,
): Column<Site>[] {
  return [
    { key: "name", header: "Name" },
    { key: "slug", header: "Slug" },
    {
      key: "liveDeployStatus",
      header: "Live Deploy",
      render: (row) => (
        <StatusBadge
          status={row.liveDeployId ? "live" : "none"}
          variantMap={SITE_STATUS_VARIANTS}
        />
      ),
    },
    {
      key: "createdAt",
      header: "Created",
      render: (row) => formatDate(row.createdAt),
    },
    {
      key: "actions",
      header: "",
      render: (row) => (
        <div className="flex gap-2">
          <button
            aria-label={`View ${row.name}`}
            onClick={() => onViewSite(row.id)}
            className="text-xs text-blue-500 hover:text-blue-600"
          >
            View
          </button>
          <button
            aria-label={`Delete ${row.name}`}
            onClick={() => onDeleteSite(row)}
            className="text-xs text-red-500 hover:text-red-600"
          >
            Delete
          </button>
        </div>
      ),
    },
  ];
}

interface SiteDetailViewProps {
  siteId: string;
  onBack: () => void;
  onSiteSaved: (site: Site) => void;
}

function SiteDetailView({ siteId, onBack, onSiteSaved }: SiteDetailViewProps) {
  const [deployPage, setDeployPage] = useState(1);
  const siteResource = useAdminResource(() => getSite(siteId));
  const deploysResource = useAdminResource(() =>
    listDeploys(siteId, { page: deployPage }),
  );

  const [siteName, setSiteName] = useState("");
  const [siteSlug, setSiteSlug] = useState("");
  const [spaMode, setSpaMode] = useState(false);
  const trimmedSiteName = siteName.trim();

  useRefreshOnPageChange(deployPage, deploysResource.refresh);

  useEffect(() => {
    if (!siteResource.data) {
      return;
    }
    setSiteName(siteResource.data.name);
    setSiteSlug(siteResource.data.slug);
    setSpaMode(siteResource.data.spaMode);
  }, [siteResource.data]);

  const handlePromoteDeploy = (deployId: string) =>
    deploysResource.runAction(async () => {
      await promoteDeploy(siteId, deployId);
    });

  const handleSaveSettings = () =>
    siteResource.runAction(async () => {
      const updatedSite = await updateSite(siteId, {
        name: trimmedSiteName,
        spaMode,
      });
      onSiteSaved(updatedSite);
    });

  const handleRollbackSite = () =>
    deploysResource.runAction(async () => {
      await rollbackDeploy(siteId);
    });

  const deployColumns = createDeployColumns(
    handlePromoteDeploy,
    deploysResource.actionLoading,
  );

  const deployData = deploysResource.data;
  const deployRows = deployData?.deploys ?? [];
  const deployTotalPages = useClampedPage(
    deployPage,
    setDeployPage,
    deployData?.totalCount,
    deployData?.perPage,
  );
  const detailError = siteResource.error ?? deploysResource.error;

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <button
          aria-label="Back to Sites"
          onClick={onBack}
          className="text-xs text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200"
        >
          Back to Sites
        </button>
      </div>

      {detailError && !siteResource.data && !deployData ? (
        <p className="text-red-600 dark:text-red-400">{detailError}</p>
      ) : (
        <>
          {detailError ? (
            <p className="mb-4 text-red-600 dark:text-red-400">{detailError}</p>
          ) : null}
          <section className="mb-6 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900 p-4">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100">
                Site Settings
              </h3>
              <button
                onClick={handleSaveSettings}
                disabled={!trimmedSiteName || siteResource.actionLoading}
                className="px-3 py-1.5 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
              >
                Save Settings
              </button>
            </div>
            {siteResource.loading && !siteResource.data ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">Loading site...</p>
            ) : (
              <div className="grid gap-3 max-w-xl">
                <label className="text-xs text-gray-600 dark:text-gray-400">
                  Name
                  <input
                    type="text"
                    value={siteName}
                    onChange={(e) => setSiteName(e.target.value)}
                    className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
                  />
                </label>
                <label className="text-xs text-gray-600 dark:text-gray-400">
                  Slug
                  <input
                    type="text"
                    value={siteSlug}
                    readOnly
                    className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-gray-100 dark:bg-gray-700 text-gray-600 dark:text-gray-300"
                  />
                </label>
                <p className="text-[11px] text-gray-500 dark:text-gray-400">
                  Slug updates are not supported in this stage.
                </p>
                <label className="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
                  <input
                    type="checkbox"
                    checked={spaMode}
                    onChange={(e) => setSpaMode(e.target.checked)}
                  />
                  SPA mode
                </label>
              </div>
            )}
          </section>

          <section>
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-base font-semibold text-gray-900 dark:text-gray-100">
                Deploy History
              </h3>
              <button
                aria-label="Rollback Site"
                onClick={handleRollbackSite}
                disabled={deploysResource.actionLoading}
                className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium disabled:opacity-50"
              >
                Rollback Site
              </button>
            </div>
            {deploysResource.loading && !deployData ? (
              <p className="text-sm text-gray-500 dark:text-gray-400">Loading deploys...</p>
            ) : (
              <AdminTable
                columns={deployColumns}
                rows={deployRows}
                rowKey="id"
                page={deployData?.page ?? deployPage}
                totalPages={deployTotalPages}
                onPageChange={setDeployPage}
                emptyMessage="No deploys found"
              />
            )}
          </section>
        </>
      )}
    </div>
  );
}

export function Sites() {
  const [viewState, setViewState] = useState<SitesViewState>({ kind: "list" });
  const [page, setPage] = useState(1);
  const [showCreateForm, setShowCreateForm] = useState(false);
  const [siteName, setSiteName] = useState("");
  const [siteSlug, setSiteSlug] = useState("");
  const [spaMode, setSpaMode] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Site | null>(null);
  const trimmedSiteName = siteName.trim();
  const trimmedSiteSlug = siteSlug.trim();

  const {
    data: siteListResult,
    setData: setSiteListResult,
    loading,
    error,
    actionLoading,
    refresh,
    runAction,
  } = useAdminResource(() => listSites({ page }));

  useRefreshOnPageChange(page, refresh);

  const resetForm = () => {
    setSiteName("");
    setSiteSlug("");
    setSpaMode(false);
  };

  const closeCreateForm = () => {
    setShowCreateForm(false);
    resetForm();
  };

  const handleCreate = () =>
    runAction(async () => {
      await createSite({
        name: trimmedSiteName,
        slug: trimmedSiteSlug,
        spaMode,
      });
      closeCreateForm();
    });

  const handleDelete = () => {
    const target = deleteTarget;
    if (!target) {
      return;
    }
    void runAction(async () => {
      await deleteSite(target.id);
      setDeleteTarget(null);
    });
  };

  const handleBackToList = () => {
    void refresh();
    setViewState({ kind: "list" });
  };

  const handleSiteSaved = (updatedSite: Site) => {
    setSiteListResult((current) => {
      if (!current) {
        return current;
      }

      return {
        ...current,
        sites: current.sites.map((site) => (site.id === updatedSite.id ? updatedSite : site)),
      };
    });
  };

  const columns = createSiteColumns(
    (siteId) => setViewState({ kind: "detail", siteId }),
    (site) => setDeleteTarget(site),
  );

  const totalPages = useClampedPage(
    page,
    setPage,
    siteListResult?.totalCount,
    siteListResult?.perPage,
  );

  if (viewState.kind === "detail") {
    return (
      <SiteDetailView
        key={viewState.siteId}
        siteId={viewState.siteId}
        onBack={handleBackToList}
        onSiteSaved={handleSiteSaved}
      />
    );
  }

  if (error && !siteListResult) {
    return (
      <div className="p-6">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100 mb-4">
          Sites
        </h2>
        <p className="text-red-600 dark:text-red-400">{error}</p>
      </div>
    );
  }

  return (
    <div className="p-6">
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">
          Sites
        </h2>
        <button
          onClick={() => setShowCreateForm(true)}
          className="px-3 py-1.5 text-xs bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 rounded hover:bg-gray-800 dark:hover:bg-gray-200 font-medium"
        >
          Add Site
        </button>
      </div>

      {error ? (
        <p className="mb-4 text-red-600 dark:text-red-400">{error}</p>
      ) : null}

      {showCreateForm && (
        <div className="mb-4 p-4 border border-gray-200 dark:border-gray-700 rounded bg-white dark:bg-gray-900">
          <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-3">
            New Site
          </h3>
          <div className="flex flex-col gap-2 max-w-md">
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Name
              <input
                type="text"
                value={siteName}
                onChange={(e) => setSiteName(e.target.value)}
                placeholder="Marketing Site"
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="text-xs text-gray-600 dark:text-gray-400">
              Slug
              <input
                type="text"
                value={siteSlug}
                onChange={(e) => setSiteSlug(e.target.value)}
                placeholder="marketing"
                className="mt-1 block w-full px-3 py-1.5 text-sm border border-gray-300 dark:border-gray-600 rounded bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
              />
            </label>
            <label className="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-400">
              <input
                type="checkbox"
                checked={spaMode}
                onChange={(e) => setSpaMode(e.target.checked)}
              />
              SPA mode
            </label>
            <div className="flex gap-2 mt-2">
              <button
                onClick={handleCreate}
                disabled={!trimmedSiteName || !trimmedSiteSlug || actionLoading}
                className="px-3 py-1.5 text-xs bg-blue-600 text-white rounded hover:bg-blue-700 font-medium disabled:opacity-50"
              >
                Create
              </button>
              <button
                onClick={closeCreateForm}
                className="px-3 py-1.5 text-xs text-gray-600 dark:text-gray-400 hover:text-gray-800 dark:hover:text-gray-200"
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      )}

      {loading ? (
        <p className="text-sm text-gray-500 dark:text-gray-400">Loading...</p>
      ) : (
        <AdminTable
          columns={columns}
          rows={siteListResult?.sites ?? []}
          rowKey="id"
          page={siteListResult?.page ?? page}
          totalPages={totalPages}
          onPageChange={setPage}
          emptyMessage="No sites configured"
        />
      )}

      <ConfirmDialog
        open={deleteTarget !== null}
        title="Delete Site"
        message={`Delete site ${deleteTarget?.name}? This cannot be undone.`}
        confirmLabel="Delete"
        onConfirm={handleDelete}
        onCancel={() => setDeleteTarget(null)}
        destructive
        loading={actionLoading}
      />
    </div>
  );
}

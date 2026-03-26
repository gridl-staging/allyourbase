import { useState, useEffect, useCallback, useRef } from "react";
import type { StorageObject } from "../types";
import {
  listStorageFiles,
  uploadStorageFile,
  deleteStorageFile,
  getSignedURL,
  storageDownloadURL,
} from "../api";
import {
  Upload,
  Trash2,
  Download,
  FolderOpen,
  X,
  Copy,
  Link2,
  Loader2,
  AlertCircle,
  HardDrive,
  Eye,
  RefreshCw,
} from "lucide-react";
import { CdnPurgeSection } from "./CdnPurgeSection";
import { cn } from "../lib/utils";
import { useAppToast } from "./ToastProvider";

const BUCKET_KEY = "ayb_storage_bucket";

type Modal =
  | { kind: "none" }
  | { kind: "delete"; file: StorageObject }
  | { kind: "preview"; file: StorageObject };

export function StorageBrowser() {
  const [bucket, setBucket] = useState(
    () => localStorage.getItem(BUCKET_KEY) || "default",
  );
  const [files, setFiles] = useState<StorageObject[]>([]);
  const [totalItems, setTotalItems] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [modal, setModal] = useState<Modal>({ kind: "none" });
  const [uploading, setUploading] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [showCdnPurge, setShowCdnPurge] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const { addToast } = useAppToast();

  const fetchFiles = useCallback(async () => {
    if (!bucket.trim()) {
      setFiles([]);
      setTotalItems(0);
      setLoading(false);
      return;
    }
    try {
      setError(null);
      setLoading(true);
      const data = await listStorageFiles(bucket);
      setFiles(data.items ?? []);
      setTotalItems(data.totalItems);
    } catch (e) {
      if (e instanceof Error && "status" in e && (e as { status: number }).status === 404) {
        setFiles([]);
        setTotalItems(0);
      } else {
        setError(e instanceof Error ? e.message : "Failed to load files");
      }
    } finally {
      setLoading(false);
    }
  }, [bucket]);

  useEffect(() => {
    localStorage.setItem(BUCKET_KEY, bucket);
    fetchFiles();
  }, [fetchFiles, bucket]);

  const handleUpload = useCallback(
    async (fileList: FileList | File[]) => {
      setUploading(true);
      setError(null);
      const filesToUpload = Array.from(fileList);
      try {
        for (const file of filesToUpload) {
          await uploadStorageFile(bucket, file);
        }
        addToast(
          "success",
          `${filesToUpload.length} file${filesToUpload.length > 1 ? "s" : ""} uploaded`,
        );
        fetchFiles();
      } catch (e) {
        addToast("error", e instanceof Error ? e.message : "Upload failed");
      } finally {
        setUploading(false);
        if (fileInput.current) fileInput.current.value = "";
      }
    },
    [bucket, fetchFiles, addToast],
  );

  const handleDelete = useCallback(
    async (file: StorageObject) => {
      try {
        await deleteStorageFile(file.bucket, file.name);
        setModal({ kind: "none" });
        setFiles((prev) => prev.filter((f) => f.id !== file.id));
        setTotalItems((prev) => prev - 1);
        addToast("success", `${file.name} deleted`);
      } catch (e) {
        addToast("error", e instanceof Error ? e.message : "Delete failed");
      }
    },
    [addToast],
  );

  const handleSignedURL = async (file: StorageObject) => {
    try {
      const { url } = await getSignedURL(file.bucket, file.name, 3600);
      navigator.clipboard.writeText(url);
      addToast("success", "Signed URL copied (1 hour)");
    } catch (e) {
      addToast("error", e instanceof Error ? e.message : "Failed to generate signed URL");
    }
  };

  const copyToClipboard = (text: string, label: string) => {
    navigator.clipboard.writeText(text);
    addToast("success", `${label} copied`);
  };

  const formatSize = (bytes: number): string => {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  };

  const isImage = (contentType: string) =>
    contentType.startsWith("image/");

  // Drag and drop handlers
  const handleDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(true);
  };

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files.length > 0) {
      handleUpload(e.dataTransfer.files);
    }
  };

  return (
    <div
      className="flex flex-col h-full"
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      {/* Toolbar */}
      <div className="px-6 py-3 border-b flex items-center gap-3">
        <h2 className="font-semibold text-sm">Storage</h2>
        <div className="flex items-center gap-2">
          <FolderOpen className="w-3.5 h-3.5 text-gray-400 dark:text-gray-500" />
          <input
            type="text"
            value={bucket}
            onChange={(e) => setBucket(e.target.value)}
            placeholder="bucket name"
            aria-label="Bucket name"
            className="px-2 py-1 text-sm border rounded w-40 focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>
        <span className="text-xs text-gray-400 dark:text-gray-500">{totalItems} {totalItems === 1 ? "file" : "files"}</span>
        <div className="ml-auto flex gap-2">
          <input
            ref={fileInput}
            type="file"
            multiple
            className="hidden"
            aria-label="Upload file"
            onChange={(e) => {
              if (e.target.files?.length) handleUpload(e.target.files);
            }}
          />
          <button
            onClick={() => fileInput.current?.click()}
            disabled={uploading || !bucket.trim()}
            className={cn(
              "inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium text-white bg-gray-900 rounded hover:bg-gray-800",
              (uploading || !bucket.trim()) && "opacity-50 cursor-not-allowed",
            )}
          >
            <Upload className="w-3.5 h-3.5" />
            {uploading ? "Uploading..." : "Upload"}
          </button>
          <button
            onClick={() => setShowCdnPurge((v) => !v)}
            className={cn(
              "inline-flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded border",
              showCdnPurge
                ? "text-blue-600 border-blue-300 bg-blue-50 dark:bg-blue-900/20"
                : "text-gray-600 dark:text-gray-300 border-gray-300 hover:bg-gray-50 dark:hover:bg-gray-700",
            )}
            title="CDN Purge"
            aria-label="CDN Purge"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            CDN Purge
          </button>
        </div>
      </div>

      {error && (
        <div className="mx-6 mt-3 px-3 py-2 text-xs text-red-700 bg-red-50 border border-red-200 rounded flex items-center gap-2">
          <AlertCircle className="w-3.5 h-3.5 shrink-0" />
          {error}
        </div>
      )}

      {/* Drag overlay */}
      {dragging && (
        <div className="absolute inset-0 z-30 bg-blue-50/80 border-2 border-dashed border-blue-400 rounded flex items-center justify-center">
          <div className="text-center">
            <Upload className="w-8 h-8 text-blue-400 mx-auto mb-2" />
            <p className="text-blue-600 text-sm font-medium">
              Drop files to upload
            </p>
          </div>
        </div>
      )}

      {/* File list */}
      <div className="flex-1 overflow-auto relative">
        {loading ? (
          <div className="flex items-center justify-center h-64 text-gray-400 dark:text-gray-500">
            <Loader2 className="w-5 h-5 animate-spin mr-2" />
            Loading files...
          </div>
        ) : !bucket.trim() ? (
          <div className="text-center py-16">
            <HardDrive className="w-10 h-10 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
            <p className="text-gray-500 dark:text-gray-400 text-sm">Enter a bucket name to browse</p>
          </div>
        ) : files.length === 0 ? (
          <div className="text-center py-16">
            <HardDrive className="w-10 h-10 text-gray-300 dark:text-gray-500 mx-auto mb-3" />
            <p className="text-gray-500 dark:text-gray-400 text-sm mb-3">
              No files in "{bucket}"
            </p>
            <button
              onClick={() => fileInput.current?.click()}
              className="text-sm text-blue-600 hover:underline"
            >
              Upload your first file
            </button>
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead className="bg-gray-50 dark:bg-gray-800 sticky top-0">
              <tr>
                <th className="text-left px-6 py-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  Name
                </th>
                <th className="text-left px-6 py-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  Type
                </th>
                <th className="text-right px-6 py-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  Size
                </th>
                <th className="text-left px-6 py-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  Created
                </th>
                <th className="text-right px-6 py-2 text-xs font-medium text-gray-500 dark:text-gray-400">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {files.map((f) => (
                <tr key={f.id} className="border-t hover:bg-gray-50 dark:hover:bg-gray-800 dark:bg-gray-800">
                  <td className="px-6 py-2">
                    <div className="flex items-center gap-1.5">
                      <span className="font-mono text-xs truncate max-w-xs">
                        {f.name}
                      </span>
                      <button
                        onClick={() =>
                          copyToClipboard(f.name, "File name")
                        }
                        className="shrink-0 p-0.5 text-gray-300 dark:text-gray-500 hover:text-gray-500 dark:hover:text-gray-300 dark:text-gray-400"
                        title="Copy name"
                        aria-label="Copy name"
                      >
                        <Copy className="w-3 h-3" />
                      </button>
                    </div>
                  </td>
                  <td className="px-6 py-2 text-xs text-gray-500 dark:text-gray-400">
                    {f.contentType}
                  </td>
                  <td className="px-6 py-2 text-xs text-gray-500 dark:text-gray-400 text-right">
                    {formatSize(f.size)}
                  </td>
                  <td className="px-6 py-2 text-xs text-gray-500 dark:text-gray-400">
                    {new Date(f.createdAt).toLocaleString()}
                  </td>
                  <td className="px-6 py-2 text-right">
                    <div className="flex gap-0.5 justify-end">
                      {isImage(f.contentType) && (
                        <button
                          onClick={() =>
                            setModal({ kind: "preview", file: f })
                          }
                          className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                          title="Preview"
                          aria-label="Preview"
                        >
                          <Eye className="w-3.5 h-3.5" />
                        </button>
                      )}
                      <a
                        href={storageDownloadURL(f.bucket, f.name)}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 inline-block"
                        title="Download"
                        aria-label="Download"
                      >
                        <Download className="w-3.5 h-3.5" />
                      </a>
                      <button
                        onClick={() => handleSignedURL(f)}
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Copy signed URL"
                        aria-label="Copy signed URL"
                      >
                        <Link2 className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() =>
                          copyToClipboard(
                            storageDownloadURL(f.bucket, f.name),
                            "Download URL",
                          )
                        }
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Copy download URL"
                        aria-label="Copy download URL"
                      >
                        <Copy className="w-3.5 h-3.5" />
                      </button>
                      <button
                        onClick={() => setModal({ kind: "delete", file: f })}
                        className="p-1 text-gray-400 dark:text-gray-500 hover:text-red-600 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                        title="Delete"
                        aria-label="Delete"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {showCdnPurge && <CdnPurgeSection />}

      {/* Preview Modal */}
      {modal.kind === "preview" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl max-w-2xl w-full mx-4">
            <div className="flex items-center justify-between px-5 py-3 border-b">
              <h3 className="font-semibold text-sm truncate">{modal.file.name}</h3>
              <button
                onClick={() => setModal({ kind: "none" })}
                className="p-1 text-gray-400 dark:text-gray-500 hover:text-gray-600 dark:hover:text-gray-300 dark:text-gray-300 rounded hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700"
                aria-label="Close"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
            <div className="p-4 flex items-center justify-center max-h-[70vh] overflow-auto">
              <img
                src={storageDownloadURL(modal.file.bucket, modal.file.name)}
                alt={modal.file.name}
                className="max-w-full max-h-[60vh] object-contain rounded"
              />
            </div>
            <div className="px-5 py-3 border-t text-xs text-gray-500 dark:text-gray-400 flex items-center gap-4">
              <span>{modal.file.contentType}</span>
              <span>{formatSize(modal.file.size)}</span>
              <span>{new Date(modal.file.createdAt).toLocaleString()}</span>
            </div>
          </div>
        </div>
      )}

      {/* Delete confirmation */}
      {modal.kind === "delete" && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
          <div className="bg-white dark:bg-gray-800 rounded-lg shadow-xl p-6 max-w-sm w-full mx-4">
            <h3 className="font-semibold mb-2">Delete File</h3>
            <p className="text-sm text-gray-600 dark:text-gray-300 mb-1">
              Are you sure? This cannot be undone.
            </p>
            <p className="text-xs font-mono text-gray-500 dark:text-gray-400 break-all mb-4">
              {modal.file.name}
            </p>
            <div className="flex justify-end gap-2">
              <button
                onClick={() => setModal({ kind: "none" })}
                className="px-3 py-1.5 text-sm text-gray-600 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700 dark:bg-gray-700 rounded border"
              >
                Cancel
              </button>
              <button
                onClick={() => handleDelete(modal.file)}
                className="px-3 py-1.5 text-sm bg-red-600 text-white rounded hover:bg-red-700"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

    </div>
  );
}

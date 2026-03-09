import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StorageBrowser } from "../StorageBrowser";
import { listStorageFiles, deleteStorageFile, uploadStorageFile } from "../../api";
import type { StorageObject } from "../../types";

vi.mock("../../api", () => ({
  listStorageFiles: vi.fn(),
  uploadStorageFile: vi.fn(),
  deleteStorageFile: vi.fn(),
  getSignedURL: vi.fn(),
  storageDownloadURL: (bucket: string, name: string) =>
    `/api/storage/${bucket}/${name}`,
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListFiles = vi.mocked(listStorageFiles);
const mockDeleteFile = vi.mocked(deleteStorageFile);
const mockUploadFile = vi.mocked(uploadStorageFile);

function makeFile(overrides: Partial<StorageObject> = {}): StorageObject {
  return {
    id: "file_1",
    bucket: "default",
    name: "photo.jpg",
    size: 1024,
    contentType: "image/jpeg",
    createdAt: "2026-01-01T00:00:00Z",
    ...overrides,
  };
}

describe("StorageBrowser", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it("shows loading state", () => {
    mockListFiles.mockReturnValue(new Promise(() => {}));
    render(<StorageBrowser />);
    expect(screen.getByText("Loading files...")).toBeInTheDocument();
  });

  it("shows empty state when no files", async () => {
    mockListFiles.mockResolvedValueOnce({ items: [], totalItems: 0 });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText(/No files in/)).toBeInTheDocument();
    });
    expect(screen.getByText("Upload your first file")).toBeInTheDocument();
  });

  it("renders file list", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [
        makeFile({ name: "image.png", size: 2048, contentType: "image/png" }),
        makeFile({ id: "file_2", name: "doc.pdf", size: 1048576, contentType: "application/pdf" }),
      ],
      totalItems: 2,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("image.png")).toBeInTheDocument();
      expect(screen.getByText("doc.pdf")).toBeInTheDocument();
    });
    expect(screen.getByText("image/png")).toBeInTheDocument();
    expect(screen.getByText("application/pdf")).toBeInTheDocument();
  });

  it("shows singular file count", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile()],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("1 file")).toBeInTheDocument();
    });
  });

  it("shows plural file count", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [
        makeFile({ id: "f1", name: "a.jpg" }),
        makeFile({ id: "f2", name: "b.jpg" }),
      ],
      totalItems: 2,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("2 files")).toBeInTheDocument();
    });
  });

  it("formats file sizes", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [
        makeFile({ id: "f1", name: "small.txt", size: 500, contentType: "text/plain" }),
        makeFile({ id: "f2", name: "medium.txt", size: 2048, contentType: "text/plain" }),
        makeFile({ id: "f3", name: "large.txt", size: 5242880, contentType: "text/plain" }),
      ],
      totalItems: 3,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("500 B")).toBeInTheDocument();
      expect(screen.getByText("2.0 KB")).toBeInTheDocument();
      expect(screen.getByText("5.0 MB")).toBeInTheDocument();
    });
  });

  it("has upload button", async () => {
    mockListFiles.mockResolvedValueOnce({ items: [], totalItems: 0 });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("Upload")).toBeInTheDocument();
    });
  });

  it("has bucket name input", async () => {
    mockListFiles.mockResolvedValueOnce({ items: [], totalItems: 0 });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByDisplayValue("default")).toBeInTheDocument();
    });
  });

  it("refetches files when bucket changes", async () => {
    const user = userEvent.setup();
    mockListFiles.mockResolvedValue({ items: [], totalItems: 0 });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(mockListFiles).toHaveBeenCalled();
    });

    const initialCallCount = mockListFiles.mock.calls.length;
    const input = screen.getByDisplayValue("default");
    await user.clear(input);
    await user.type(input, "images");
    await waitFor(() => {
      expect(mockListFiles.mock.calls.length).toBeGreaterThan(initialCallCount);
    });
  });

  it("opens delete confirmation", async () => {
    const user = userEvent.setup();
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile({ name: "delete-me.jpg" })],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("delete-me.jpg")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle("Delete");
    await user.click(deleteButtons[0]);
    expect(screen.getByText("Delete File")).toBeInTheDocument();
  });

  it("deletes a file on confirm", async () => {
    const user = userEvent.setup();
    mockListFiles.mockResolvedValue({
      items: [makeFile()],
      totalItems: 1,
    });
    mockDeleteFile.mockResolvedValueOnce();
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("photo.jpg")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle("Delete");
    await user.click(deleteButtons[0]);

    // Find the red "Delete" button in the confirmation modal.
    const confirmDeleteBtn = screen.getAllByRole("button", { name: "Delete" }).find(
      (btn) => btn.classList.contains("bg-red-600"),
    );
    expect(confirmDeleteBtn).toBeDefined();
    await user.click(confirmDeleteBtn!);

    await waitFor(() => {
      expect(mockDeleteFile).toHaveBeenCalledWith("default", "photo.jpg");
    });
  });

  it("closes delete modal on Cancel", async () => {
    const user = userEvent.setup();
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile()],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("photo.jpg")).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByTitle("Delete");
    await user.click(deleteButtons[0]);
    expect(screen.getByText("Delete File")).toBeInTheDocument();

    await user.click(screen.getByText("Cancel"));
    expect(screen.queryByText("Delete File")).not.toBeInTheDocument();
  });

  it("has download links", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile({ name: "dl-me.jpg" })],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("dl-me.jpg")).toBeInTheDocument();
    });

    const downloadLinks = screen.getAllByTitle("Download");
    expect(downloadLinks[0]).toHaveAttribute(
      "href",
      "/api/storage/default/dl-me.jpg",
    );
  });

  it("displays error on fetch failure", async () => {
    mockListFiles.mockRejectedValueOnce(new Error("network error"));
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("network error")).toBeInTheDocument();
    });
  });

  it("shows preview button for image files", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile({ contentType: "image/png", name: "pic.png" })],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByTitle("Preview")).toBeInTheDocument();
    });
  });

  it("does not show preview button for non-image files", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile({ contentType: "application/pdf", name: "doc.pdf" })],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("doc.pdf")).toBeInTheDocument();
    });
    expect(screen.queryByTitle("Preview")).not.toBeInTheDocument();
  });

  it("has signed URL and copy URL buttons", async () => {
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile()],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByTitle("Copy signed URL")).toBeInTheDocument();
      expect(screen.getByTitle("Copy download URL")).toBeInTheDocument();
    });
  });

  it("persists bucket to localStorage", async () => {
    mockListFiles.mockResolvedValue({ items: [], totalItems: 0 });
    render(<StorageBrowser />);
    const user = userEvent.setup();

    const input = screen.getByPlaceholderText("bucket name");
    await user.clear(input);
    await user.type(input, "uploads");

    expect(localStorage.getItem("ayb_storage_bucket")).toBe("uploads");
  });

  it("uploads a file via file input", async () => {
    const user = userEvent.setup();
    mockListFiles.mockResolvedValue({ items: [], totalItems: 0 });
    mockUploadFile.mockResolvedValueOnce(makeFile({ name: "new.jpg" }));
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText("Upload")).toBeInTheDocument();
    });

    const file = new File(["hello"], "new.jpg", { type: "image/jpeg" });
    const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement;
    expect(fileInput).not.toBeNull();
    await user.upload(fileInput, file);

    await waitFor(() => {
      expect(mockUploadFile).toHaveBeenCalledWith("default", file);
    });
  });

  it("treats 404 as empty bucket", async () => {
    const err = new Error("Not Found");
    Object.assign(err, { status: 404 });
    mockListFiles.mockRejectedValueOnce(err);
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByText(/No files in/)).toBeInTheDocument();
    });
    // No error message should be shown for 404.
    expect(screen.queryByText("Not Found")).not.toBeInTheDocument();
  });

  it("opens preview modal for image files", async () => {
    const user = userEvent.setup();
    mockListFiles.mockResolvedValueOnce({
      items: [makeFile({ contentType: "image/png", name: "preview.png" })],
      totalItems: 1,
    });
    render(<StorageBrowser />);
    await waitFor(() => {
      expect(screen.getByTitle("Preview")).toBeInTheDocument();
    });

    await user.click(screen.getByTitle("Preview"));
    // The img element should appear with the correct src.
    const img = document.querySelector("img");
    expect(img).not.toBeNull();
    expect(img!.getAttribute("src")).toBe("/api/storage/default/preview.png");
    expect(img!.getAttribute("alt")).toBe("preview.png");
  });
});

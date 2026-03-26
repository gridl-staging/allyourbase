import { useEffect, useMemo, useState } from "react";
import { Maximize2, Minus, Plus, RotateCw } from "lucide-react";
import type { SchemaCache } from "../types";
import { useSchemaDesignerData } from "./schema-designer/useSchemaDesignerData";
import { cn } from "../lib/utils";

interface SchemaDesignerProps {
  schema?: SchemaCache;
  loading?: boolean;
  error?: string | null;
  onRetry?: () => void;
  onAutoArrange?: () => void;
}

export function SchemaDesigner({
  schema,
  loading,
  error,
  onRetry,
  onAutoArrange,
}: SchemaDesignerProps) {
  const {
    nodes,
    edges,
    detailsByTableId,
    loading: dataLoading,
    error: dataError,
    retry,
  } = useSchemaDesignerData({ initialSchema: schema });
  const [selectedTableId, setSelectedTableId] = useState<string | null>(nodes[0]?.tableId ?? null);
  const [zoom, setZoom] = useState(1);
  const [arranged, setArranged] = useState(false);

  const selected = selectedTableId ? detailsByTableId[selectedTableId] : null;

  useEffect(() => {
    if (!selectedTableId && nodes[0]) {
      setSelectedTableId(nodes[0].tableId);
    }
  }, [nodes, selectedTableId]);

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const fromUrl = params.get("schemaTable");
    if (fromUrl && detailsByTableId[fromUrl]) {
      setSelectedTableId(fromUrl);
    }
    // Run once at mount with current schema graph.
  }, []);

  useEffect(() => {
    if (!selectedTableId) return;
    const params = new URLSearchParams(window.location.search);
    params.set("schemaTable", selectedTableId);
    const next = `${window.location.pathname}?${params.toString()}${window.location.hash}`;
    window.history.replaceState(null, "", next);
  }, [selectedTableId]);

  const arrangedNodes = useMemo(() => {
    if (!arranged) return nodes;
    return nodes.map((node, idx) => {
      const row = Math.floor(idx / 3);
      const col = idx % 3;
      return {
        ...node,
        position: { x: col * 330, y: row * 230 },
      };
    });
  }, [arranged, nodes]);

  const effectiveLoading = loading ?? dataLoading;
  const effectiveError = error ?? dataError;
  const effectiveRetry = onRetry ?? retry;

  if (effectiveLoading) {
    return <div className="p-6 text-sm text-gray-500 dark:text-gray-400">Loading schema designer...</div>;
  }

  if (effectiveError) {
    return (
      <div className="p-6 space-y-3">
        <p className="text-sm text-red-600 dark:text-red-400">{effectiveError}</p>
        {effectiveRetry && (
          <button
            onClick={effectiveRetry}
            className="px-3 py-1.5 text-sm rounded bg-red-600 text-white hover:bg-red-700"
          >
            Retry
          </button>
        )}
      </div>
    );
  }

  if (arrangedNodes.length === 0) {
    return <div className="p-6 text-sm text-gray-500 dark:text-gray-400">No tables available</div>;
  }

  return (
    <div className="h-full flex flex-col">
      <div className="px-4 py-3 border-b border-gray-200 dark:border-gray-700 flex items-center justify-between">
        <h2 className="text-sm font-semibold text-gray-800 dark:text-gray-100">Schema Designer</h2>
        <div className="flex items-center gap-2">
          <button
            aria-label="Zoom Out"
            onClick={() => setZoom((z) => Math.max(0.5, z - 0.1))}
            className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 text-xs"
          >
            <Minus className="w-3 h-3" />
          </button>
          <span data-testid="schema-zoom-level" className="text-xs text-gray-500 dark:text-gray-400 w-10 text-center">
            {Math.round(zoom * 100)}%
          </span>
          <button
            aria-label="Zoom In"
            onClick={() => setZoom((z) => Math.min(2, z + 0.1))}
            className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 text-xs"
          >
            <Plus className="w-3 h-3" />
          </button>
          <button
            aria-label="Fit View"
            onClick={() => setZoom(1)}
            className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 text-xs inline-flex items-center gap-1"
          >
            <Maximize2 className="w-3 h-3" />
            Fit
          </button>
          <button
            aria-label="Auto Arrange"
            onClick={() => {
              setArranged(true);
              onAutoArrange?.();
            }}
            className="px-2 py-1 rounded border border-gray-200 dark:border-gray-700 text-xs inline-flex items-center gap-1"
          >
            <RotateCw className="w-3 h-3" />
            Auto Arrange
          </button>
        </div>
      </div>

      <div className="flex-1 min-h-0 grid grid-cols-12">
        <div className="col-span-8 overflow-auto border-r border-gray-200 dark:border-gray-700">
          <div className="relative min-h-[540px] p-6" style={{ transform: `scale(${zoom})`, transformOrigin: "top left" }}>
            <svg className="absolute inset-0 w-full h-full pointer-events-none" data-testid="schema-edges">
              {edges.map((edge) => {
                const s = arrangedNodes.find((n) => n.id === edge.source);
                const t = arrangedNodes.find((n) => n.id === edge.target);
                if (!s || !t) return null;
                const x1 = s.position.x + 220;
                const y1 = s.position.y + 50;
                const x2 = t.position.x + 20;
                const y2 = t.position.y + 50;
                const mx = (x1 + x2) / 2;
                const my = (y1 + y2) / 2;
                return (
                  <g key={edge.id}>
                    <line x1={x1} y1={y1} x2={x2} y2={y2} stroke="#94a3b8" strokeWidth="1.5" />
                    <text x={mx} y={my - 4} fontSize="10" fill="#64748b">{edge.label}</text>
                  </g>
                );
              })}
            </svg>

            {arrangedNodes.map((node) => {
              const isSelected = selectedTableId === node.tableId;
              return (
                <button
                  key={node.id}
                  data-testid={`schema-node-${node.id}`}
                  onClick={() => setSelectedTableId(node.tableId)}
                  className={cn(
                    "absolute text-left w-[240px] rounded-lg border bg-white dark:bg-gray-900 p-3 shadow-sm",
                    isSelected
                      ? "border-blue-500 dark:border-blue-400"
                      : "border-gray-200 dark:border-gray-700",
                  )}
                  style={{ left: node.position.x, top: node.position.y }}
                >
                  <div className="text-xs font-semibold text-gray-900 dark:text-gray-100">{node.label}</div>
                  <div className="text-[10px] text-gray-500 dark:text-gray-400 mb-2">{node.kind} · {node.columnCount} cols</div>
                  <div className="space-y-0.5">
                    {node.columnsPreview.map((c) => (
                      <div key={c} className="text-[10px] text-gray-600 dark:text-gray-300 truncate">{c}</div>
                    ))}
                  </div>
                </button>
              );
            })}
          </div>
        </div>

        <aside data-testid="schema-details-panel" className="col-span-4 overflow-auto p-4 space-y-3">
          {selected ? (
            <>
              <h3 className="text-sm font-semibold text-gray-900 dark:text-gray-100">{selected.schema}.{selected.name}</h3>
              <p className="text-xs text-gray-500 dark:text-gray-400">{selected.kind}</p>

              <section>
                <h4 className="text-xs font-semibold text-gray-700 dark:text-gray-200 mb-1">Columns</h4>
                <ul className="space-y-1">
                  {selected.columns.map((c) => (
                    <li key={c.name} className="text-xs text-gray-600 dark:text-gray-300">
                      {c.name} <span className="text-gray-400">({c.type})</span>
                    </li>
                  ))}
                </ul>
              </section>

              <section>
                <h4 className="text-xs font-semibold text-gray-700 dark:text-gray-200 mb-1">Foreign Keys</h4>
                {(selected.foreignKeys?.length ?? 0) === 0 ? (
                  <p className="text-xs text-gray-400">None</p>
                ) : (
                  <ul className="space-y-1">
                    {selected.foreignKeys.map((fk) => (
                      <li key={fk.constraintName} className="text-xs text-gray-600 dark:text-gray-300">{fk.constraintName}</li>
                    ))}
                  </ul>
                )}
              </section>

              <section>
                <h4 className="text-xs font-semibold text-gray-700 dark:text-gray-200 mb-1">Indexes</h4>
                {selected.indexes.length === 0 ? (
                  <p className="text-xs text-gray-400">None</p>
                ) : (
                  <ul className="space-y-1">
                    {selected.indexes.map((idx) => (
                      <li key={idx.name} className="text-xs text-gray-600 dark:text-gray-300">{idx.name}</li>
                    ))}
                  </ul>
                )}
              </section>
            </>
          ) : (
            <p className="text-xs text-gray-500 dark:text-gray-400">Select a table node to inspect details.</p>
          )}
        </aside>
      </div>
    </div>
  );
}

import { useState } from "react";
import { ArrowLeft, Loader2, AlertCircle } from "lucide-react";
import CodeMirror from "@uiw/react-codemirror";
import { javascript } from "@codemirror/lang-javascript";
import { deployEdgeFunction } from "../../api";
import { DEFAULT_SOURCE } from "./helpers";
import { useCodeMirrorTheme } from "../codeMirrorTheme";

interface FunctionCreateProps {
  onBack: () => void;
  addToast: (type: "success" | "error", message: string) => void;
}

export function FunctionCreate({ onBack, addToast }: FunctionCreateProps) {
  const [name, setName] = useState("");
  const [source, setSource] = useState(DEFAULT_SOURCE);
  const [entryPoint, setEntryPoint] = useState("handler");
  const [timeoutMs, setTimeoutMs] = useState(5000);
  const [isPublic, setIsPublic] = useState(true);
  const [deploying, setDeploying] = useState(false);
  const [deployError, setDeployError] = useState<string | null>(null);
  const codeMirrorTheme = useCodeMirrorTheme();

  const handleDeploy = async () => {
    if (!name.trim()) return;
    setDeploying(true);
    setDeployError(null);
    try {
      await deployEdgeFunction({
        name: name.trim(),
        source,
        entry_point: entryPoint,
        timeout_ms: timeoutMs,
        public: isPublic,
      });
      addToast("success", `Function "${name}" deployed`);
      onBack();
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Deploy failed";
      if (msg.toLowerCase().includes("compile") || msg.toLowerCase().includes("transpil") || msg.toLowerCase().includes("syntax")) {
        setDeployError(msg);
      }
      addToast("error", msg);
      setDeploying(false);
    }
  };

  return (
    <div className="p-6 text-gray-900 dark:text-gray-100">
      <div className="flex items-center gap-3 mb-6">
        <button
          onClick={onBack}
          className="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-700"
          aria-label="Back"
        >
          <ArrowLeft className="w-4 h-4" />
        </button>
        <h1 className="text-xl font-semibold">Deploy New Function</h1>
      </div>

      <div className="space-y-4 max-w-3xl">
        <div>
          <label htmlFor="fn-name" className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">
            Name
          </label>
          <input
            id="fn-name"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-function"
            className="w-full px-3 py-1.5 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
          />
        </div>

        {deployError && (
          <div
            className="flex items-start gap-2 px-3 py-2 bg-red-50 border border-red-200 rounded text-sm text-red-800"
            data-testid="deploy-error"
          >
            <AlertCircle className="w-4 h-4 shrink-0 mt-0.5" />
            <div>
              <span className="font-medium">Deploy error: </span>
              <span>{deployError}</span>
            </div>
          </div>
        )}

        <div>
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Source</label>
          <CodeMirror
            value={source}
            onChange={(val) => {
              setSource(val);
              if (deployError) setDeployError(null);
            }}
            extensions={[javascript({ typescript: true })]}
            theme={codeMirrorTheme}
            data-testid="codemirror-editor"
            className="border border-gray-300 dark:border-gray-600 rounded overflow-hidden"
            height="300px"
          />
        </div>

        <div className="grid grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Entry Point</label>
            <input
              type="text"
              value={entryPoint}
              onChange={(e) => setEntryPoint(e.target.value)}
              className="w-full px-3 py-1.5 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 dark:text-gray-200 mb-1">Timeout (ms)</label>
            <input
              type="number"
              value={timeoutMs}
              onChange={(e) => setTimeoutMs(Number(e.target.value))}
              className="w-full px-3 py-1.5 border border-gray-300 dark:border-gray-600 rounded text-sm bg-white dark:bg-gray-800 text-gray-900 dark:text-gray-100"
            />
          </div>
          <div className="flex items-end gap-2 pb-0.5">
            <label className="flex items-center gap-2 text-sm cursor-pointer">
              <input
                type="checkbox"
                checked={isPublic}
                onChange={(e) => setIsPublic(e.target.checked)}
              />
              Public
            </label>
          </div>
        </div>

        <div className="pt-2 border-t">
          <button
            onClick={handleDeploy}
            disabled={deploying || !name.trim()}
            className="flex items-center gap-1.5 px-4 py-1.5 bg-gray-900 text-white rounded text-sm hover:bg-gray-800 disabled:opacity-50"
          >
            {deploying && <Loader2 className="w-3.5 h-3.5 animate-spin" />}
            Deploy
          </button>
        </div>
      </div>
    </div>
  );
}

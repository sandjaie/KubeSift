/*
Purpose: Review one fixed-target Helm action in the existing modal and diff presentation.
Used by: HelmWorkspace through App.
Responsibilities: Resolve pinned charts, edit complete overrides and claim one fully loaded review token.
Important: Navigation revokes drafts; confirmation never accepts replacement target or chart inputs.
*/
import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { ChartEntry, ChartSnapshot, Options, Review, Target } from "../../bindings/internal/helmmanager/models";
import type { RecordRef, ReleaseSummary } from "../../bindings/internal/helmreader/models";
import type { HelmSource } from "../../bindings/internal/preferences/models";
import { message } from "../connections/api";
import { reviewOwner } from "../operations/reviewState";
import { YAMLDiff } from "../operations/YAMLDiff";
import { Modal } from "../ui/ReviewDialog";
import { readHistory } from "./api";
import { canConfirmHelm, helmActionLabels, type HelmAction } from "./managementState";
import {
  helmManagementAPI as api,
  importHelmValues,
  listHelmCharts,
  loadUpgradeValues,
  prepareHelmReview,
  resolveHelmChart,
} from "./managementApi";

type LoadedReview = Review & { content: Record<string, string> };
const discard = (id: string) => {
  void api.DiscardHelmDraft(id).catch(() => {});
};

export function HelmActionDialog({
  action,
  target,
  contextLabel,
  revision,
  valid,
  onClose,
  onConfirm,
}: {
  action: HelmAction;
  target: Target;
  contextLabel: string;
  revision?: RecordRef;
  valid: boolean;
  onClose: () => void;
  onConfirm: (token: string) => void;
}) {
  const needsChart = action === "install" || action === "upgrade";
  const [name, setName] = useState(target.releaseName);
  const [namespace, setNamespace] = useState(target.namespace);
  const [sources, setSources] = useState<HelmSource[]>([]);
  const [sourceID, setSourceID] = useState("");
  const [chartName, setChartName] = useState("");
  const [version, setVersion] = useState("");
  const [referenceKind, setReferenceKind] = useState<"version" | "digest">("version");
  const [entries, setEntries] = useState<ChartEntry[]>([]);
  const [cursor, setCursor] = useState("");
  const [chart, setChart] = useState<ChartSnapshot>();
  const [values, setValues] = useState("{}\n");
  const [valuesReady, setValuesReady] = useState(action !== "upgrade");
  const [history, setHistory] = useState<ReleaseSummary[]>([]);
  const [historyPage, setHistoryPage] = useState(0);
  const [historyTotal, setHistoryTotal] = useState(0);
  const [rollback, setRollback] = useState<RecordRef>();
  const [options, setOptions] = useState<Options>({ timeoutSeconds: 300, disableHooks: false, createNamespace: false, keepHistory: false });
  const [review, setReview] = useState<LoadedReview>();
  const [busy, setBusy] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [expired, setExpired] = useState(false);
  const [owner] = useState(() => reviewOwner(discard));
  const chartID = useRef("");
  const active = useRef(true);
  const cancelledRead = useRef<(() => void) | undefined>();
  const cancelInitialValues = useRef<(() => void) | undefined>();
  const source = sources.find((item) => item.id === sourceID);

  useLayoutEffect(() => {
    active.current = valid;
    if (!valid) {
      owner.abandon();
      cancelledRead.current?.();
      if (chartID.current) discard(chartID.current);
      chartID.current = "";
      setChart(undefined);
      setReview(undefined);
      setBusy(false);
    }
    return () => {
      active.current = false;
      owner.abandon();
      cancelledRead.current?.();
      if (chartID.current) discard(chartID.current);
      chartID.current = "";
    };
  }, [valid, owner]);

  useEffect(() => {
    let current = true;
    void api.ListHelmSources().then(
      (items) => {
        if (current) setSources(items ?? []);
      },
      (reason: unknown) => {
        if (current) setError(message(reason));
      },
    );
    return () => {
      current = false;
    };
  }, []);

  useEffect(() => {
    if (action !== "upgrade" || !valid) return;
    let current = true;
    const read = loadUpgradeValues(target);
    cancelInitialValues.current = () => {
      current = false;
      read.cancel();
    };
    void read.promise.then(
      (text) => {
        if (current) {
          setValues(text);
          setValuesReady(true);
        }
      },
      (reason: unknown) => {
        if (current) setError(message(reason));
      },
    );
    return () => {
      current = false;
      read.cancel();
    };
  }, [action, valid]);

  useEffect(() => {
    if (action !== "rollback" || !revision || !valid) return;
    let current = true;
    const read = readHistory({ record: revision, page: historyPage });
    void read.promise.then(
      (page) => {
        if (!current) return;
        setHistory(page.items ?? []);
        setHistoryTotal(page.total);
        if (historyPage === 0 && page.items?.[0]?.record.revision !== revision.revision) setRollback(revision);
      },
      (reason: unknown) => {
        if (current) setError(message(reason));
      },
    );
    return () => {
      current = false;
      read.cancel();
    };
  }, [action, historyPage, valid]);

  useEffect(() => {
    setExpired(false);
    if (!review) return;
    const timer = window.setTimeout(
      () => {
        setExpired(true);
        owner.abandon();
      },
      Math.max(0, Date.parse(review.expiresAt) - Date.now()),
    );
    return () => window.clearTimeout(timer);
  }, [review, owner]);

  function clearChart() {
    if (chartID.current) discard(chartID.current);
    chartID.current = "";
    setChart(undefined);
  }
  function retainChart(value: ChartSnapshot) {
    if (!active.current) {
      discard(value.id);
      return;
    }
    clearChart();
    chartID.current = value.id;
    setChart(value);
  }
  async function resolveChart(local: boolean) {
    if (busy) return;
    setBusy(true);
    setError("");
    let handle = "";
    try {
      if (local) handle = await api.PickHelmChart();
      if (!active.current) {
        if (handle) discard(handle);
        return;
      }
      const request = resolveHelmChart({
        sourceID: local ? "" : sourceID,
        localHandle: handle,
        chart: local ? "" : chartName,
        version: local || referenceKind === "digest" ? "" : version,
        digest: local || referenceKind === "version" ? "" : version,
      });
      cancelledRead.current = request.cancel;
      retainChart(await request.promise);
    } catch (reason) {
      if (handle) discard(handle);
      if (active.current) setError(message(reason));
    } finally {
      if (active.current) setBusy(false);
    }
  }
  async function search(next = false) {
    setBusy(true);
    setError("");
    const request = listHelmCharts(sourceID, chartName, next ? cursor : "");
    cancelledRead.current = request.cancel;
    try {
      const page = await request.promise;
      if (active.current) {
        setEntries(page.items ?? []);
        setCursor(page.nextCursor);
      }
    } catch (reason) {
      if (active.current) setError(message(reason));
    } finally {
      if (active.current) setBusy(false);
    }
  }
  function prepare() {
    if (!valid || busy || !owner.canPrepare()) return;
    setBusy(true);
    setError("");
    owner.prepare(
      prepareHelmReview({
        action,
        target: { ...target, namespace, releaseName: name },
        chartID: chart?.id ?? "",
        valuesYAML: needsChart ? values : "",
        rollbackRecord: rollback ?? null,
        options,
      }),
      (value) => {
        chartID.current = "";
        setReview(value);
        setBusy(false);
      },
      (reason) => {
        clearChart();
        setBusy(false);
        setError(message(reason));
      },
    );
  }
  async function edit() {
    if (!review || busy) return;
    setBusy(true);
    try {
      if (needsChart) retainChart(await api.ReviseHelmReview(review.token));
      owner.abandon();
      owner.resume();
      setReview(undefined);
      setError("");
    } catch (reason) {
      setError(message(reason));
    } finally {
      setBusy(false);
    }
  }

  if (!valid) return null;
  return (
    <Modal title={`${helmActionLabels[action]} Helm release`} onCancel={onClose}>
      <p className="frozen-target">
        {contextLabel} / {namespace} / {name || "new release"} · {target.driver} storage
      </p>
      <p className="muted">This target is fixed. Navigation invalidates this draft. Values can contain sensitive content.</p>
      {error && <p role="alert">{error}</p>}
      {busy && <p role="status">{review ? "Updating draft…" : "Preparing native Helm inputs…"}</p>}
      {!review ? (
        <>
          <fieldset disabled={busy || submitting} className="helm-action-fields">
            <legend>Action inputs</legend>
            {action === "install" && (
              <>
                <label>
                  Release name
                  <input value={name} maxLength={53} onChange={(event) => setName(event.target.value)} />
                </label>
                <label>
                  Target namespace
                  <input value={namespace} maxLength={63} onChange={(event) => setNamespace(event.target.value)} />
                </label>
              </>
            )}
            {needsChart && (
              <>
                <label>
                  Chart source
                  <select
                    value={sourceID}
                    onChange={(event) => {
                      clearChart();
                      setSourceID(event.target.value);
                      setEntries([]);
                      setCursor("");
                      setChartName("");
                      setVersion("");
                      setReferenceKind("version");
                    }}
                  >
                    <option value="">Local packaged chart</option>
                    {sources.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.name}
                      </option>
                    ))}
                  </select>
                </label>
                {!sourceID ? (
                  <button onClick={() => void resolveChart(true)}>Select packaged chart</button>
                ) : (
                  <>
                    <label>
                      Chart name
                      <input
                        value={chartName}
                        spellCheck={false}
                        onChange={(event) => {
                          clearChart();
                          setChartName(event.target.value);
                          setCursor("");
                        }}
                      />
                    </label>
                    {source?.kind === "https" && (
                      <div className="button-row">
                        <button onClick={() => void search()}>Search chart versions</button>
                        {cursor && <button onClick={() => void search(true)}>Next chart page</button>}
                        <select
                          aria-label="Repository chart version"
                          value=""
                          onChange={(event) => {
                            const entry = entries[Number(event.target.value)];
                            if (!entry) return;
                            clearChart();
                            setChartName(entry.name);
                            setVersion(entry.version);
                          }}
                        >
                          <option value="">Choose an exact version</option>
                          {entries.map((entry, index) => (
                            <option key={`${entry.name}-${entry.version}`} value={index}>
                              {entry.name} · {entry.version}
                            </option>
                          ))}
                        </select>
                      </div>
                    )}
                    {source?.kind === "oci" && (
                      <label>
                        OCI reference
                        <select
                          value={referenceKind}
                          onChange={(event) => {
                            clearChart();
                            setReferenceKind(event.target.value as "version" | "digest");
                            setVersion("");
                          }}
                        >
                          <option value="version">Exact version</option>
                          <option value="digest">Manifest digest</option>
                        </select>
                      </label>
                    )}
                    <label>
                      {referenceKind === "digest" ? "OCI digest" : "Exact chart version"}
                      <input
                        value={version}
                        spellCheck={false}
                        onChange={(event) => {
                          clearChart();
                          setVersion(event.target.value);
                        }}
                      />
                    </label>
                    <button disabled={!chartName || !version} onClick={() => void resolveChart(false)}>
                      Resolve chart
                    </button>
                  </>
                )}
                <p className="muted">Configure private sources and session credentials in Settings. Chart dependencies must be packaged.</p>
                {chart && (
                  <p role="status">
                    Selected {chart.name} {chart.version}
                    <br />
                    <code>{chart.digest}</code>
                  </p>
                )}
                <label>
                  Complete override values
                  <textarea
                    aria-label="Override values"
                    className="yaml-view edit-source"
                    value={values}
                    spellCheck={false}
                    disabled={!valuesReady}
                    onChange={(event) => setValues(event.target.value)}
                  />
                </label>
                <div className="button-row">
                  <button
                    onClick={() => {
                      cancelInitialValues.current?.();
                      setValues("{}\n");
                      setValuesReady(true);
                    }}
                  >
                    Reset to chart defaults
                  </button>
                  <button
                    onClick={() => {
                      cancelInitialValues.current?.();
                      setBusy(true);
                      void importHelmValues()
                        .then(
                          (text) => {
                            if (active.current) {
                              setValues(text);
                              setValuesReady(true);
                            }
                          },
                          (reason: unknown) => {
                            if (active.current) setError(message(reason));
                          },
                        )
                        .finally(() => {
                          if (active.current) setBusy(false);
                        });
                    }}
                  >
                    Import values file
                  </button>
                </div>
                {action === "upgrade" && (
                  <p className="muted">
                    Loaded from the current release, not the inspected historical revision. These values replace the entire old override
                    mapping.
                  </p>
                )}
              </>
            )}
            {action === "rollback" && (
              <>
                <label>
                  Retained revision
                  <select
                    aria-label="Rollback revision"
                    value={rollback?.revision ?? ""}
                    onChange={(event) => setRollback(history.find((row) => row.record.revision === event.target.value)?.record)}
                  >
                    <option value="">Choose an earlier revision</option>
                    {history.map((row) => (
                      <option disabled={!!row.problem} key={row.record.recordName} value={row.record.revision}>
                        Revision {row.record.revision} · {row.chartName} {row.chartVersion} · {row.status}
                      </option>
                    ))}
                  </select>
                </label>
                <div className="button-row">
                  <button disabled={historyPage === 0} onClick={() => setHistoryPage(historyPage - 1)}>
                    Previous revisions
                  </button>
                  <button disabled={(historyPage + 1) * 100 >= historyTotal} onClick={() => setHistoryPage(historyPage + 1)}>
                    Next revisions
                  </button>
                </div>
                <p>Rollback restores recorded Kubernetes release configuration, not databases or external side effects.</p>
              </>
            )}
            <label>
              <input
                type="checkbox"
                checked={options.disableHooks}
                onChange={(event) => setOptions({ ...options, disableHooks: event.target.checked })}
              />{" "}
              Disable hooks for this action
            </label>
            {action === "install" && (
              <label>
                <input
                  type="checkbox"
                  checked={options.createNamespace}
                  onChange={(event) => setOptions({ ...options, createNamespace: event.target.checked })}
                />{" "}
                Create target namespace if missing
              </label>
            )}
            {action === "uninstall" && (
              <label>
                <input
                  type="checkbox"
                  checked={options.keepHistory}
                  onChange={(event) => setOptions({ ...options, keepHistory: event.target.checked })}
                />{" "}
                Keep release history
              </label>
            )}
            <label>
              Timeout in minutes
              <input
                type="number"
                min={1}
                max={30}
                value={options.timeoutSeconds / 60}
                onChange={(event) => setOptions({ ...options, timeoutSeconds: Number(event.target.value) * 60 })}
              />
            </label>
            <p className="muted">
              Wait for readiness and Jobs is enabled. The timeout is also the overall deadline. No automatic rollback or cleanup.
            </p>
          </fieldset>
          <button
            className="primary"
            disabled={busy || !name || !namespace || (needsChart && (!chart || !valuesReady)) || (action === "rollback" && !rollback)}
            onClick={prepare}
          >
            Prepare review
          </button>
        </>
      ) : (
        <>
          <h3>Helm preview</h3>
          <p>Source: {source ? `${source.name} · ${source.url}` : needsChart ? "Local packaged chart" : "Captured release storage"}</p>
          <p>
            {review.chartName} {review.chartVersion} · {review.current ? `Current revision ${review.current.revision}` : "New release"}
            {review.rollback && ` → retained revision ${review.rollback.revision}`}
          </p>
          {review.digest && (
            <p>
              <code>{review.digest}</code> · Content hash, not publisher verification
            </p>
          )}
          <p>
            Hooks: {review.options.disableHooks ? "disabled" : "enabled"} · timeout {review.options.timeoutSeconds / 60} minutes · namespace
            creation {review.options.createNamespace ? "enabled" : "disabled"} · keep history{" "}
            {review.options.keepHistory ? "enabled" : "disabled"}
          </p>
          <p>
            Apply behavior:{" "}
            {action === "install" ? "Helm SDK default" : action === "upgrade" || action === "rollback" ? "auto" : "background deletion"}.
          </p>
          {review.warnings?.map((warning) => (
            <p className="warning" key={warning}>
              {warning}
            </p>
          ))}
          {(needsChart || action === "rollback") && (
            <details open>
              <summary>Complete override values difference</summary>
              <YAMLDiff before={review.content.currentValues ?? ""} after={review.content.values ?? ""} />
            </details>
          )}
          <details open>
            <summary>Stored current → proposed manifest</summary>
            <YAMLDiff before={review.content.currentManifest ?? ""} after={review.content.manifest ?? ""} />
          </details>
          {(["resources", "hooks", "crds"] as const).map((section) => (
            <details key={section}>
              <summary>
                {section === "crds"
                  ? "CRDs (retained after uninstall)"
                  : section === "hooks"
                    ? "Hook impact"
                    : "Resources and namespace impact"}
              </summary>
              <pre className="yaml-view" tabIndex={0}>
                {review.content[section] || "None recorded."}
              </pre>
            </details>
          ))}
          {expired && <p role="alert">Review expired. Close this draft and prepare again.</p>}
          <div className="button-row">
            <button disabled={busy || submitting || expired} onClick={() => void edit()}>
              Edit inputs
            </button>
            <button
              className="primary"
              disabled={busy || expired || !canConfirmHelm(review, valid, submitting)}
              onClick={() => {
                if (!canConfirmHelm(review, valid, submitting) || !owner.claim(review.token)) return;
                setSubmitting(true);
                onConfirm(review.token);
              }}
            >
              Confirm {action === "rollback" ? "rollback" : action}
            </button>
          </div>
        </>
      )}
    </Modal>
  );
}

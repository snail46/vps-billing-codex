import type { Operation } from "@vps-billing/api-types";

interface OperationProgressProps {
  operation: Operation;
  translate: (key: string) => string;
}

export function OperationProgress({ operation, translate }: OperationProgressProps) {
  const statusKey = operation.message_key ?? `operation.${operation.status}`;

  return (
    <section className="operation-progress" aria-live="polite" aria-busy={!isTerminal(operation.status)}>
      <header className="operation-progress__header">
        <strong>{translate(statusKey)}</strong>
        <span>{operation.progress}%</span>
      </header>
      <progress max={100} value={operation.progress} aria-label={translate("operation.progress")} />
      {operation.status === "retrying" ? (
        <p className="operation-progress__retry">
          {translate("operation.retryAttempt")} {operation.retry_count}/{operation.max_retries}
        </p>
      ) : null}
      <ol className="operation-progress__steps">
        {operation.steps.map((step) => (
          <li key={step.key} data-status={step.status}>
            <span>{translate(`operation.steps.${step.key}`)}</span>
            <span>{translate(`operation.stepStatus.${step.status}`)}</span>
          </li>
        ))}
      </ol>
      {operation.error_code ? (
        <p className="operation-progress__error" role="alert">
          {translate("operation.errorCode")}: {operation.error_code}
        </p>
      ) : null}
    </section>
  );
}

function isTerminal(status: Operation["status"]): boolean {
  return status === "succeeded" || status === "failed" || status === "cancelled";
}

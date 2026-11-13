/*
Purpose: Choose one explicit namespace through a searchable dropdown.
Used by: The active workspace identity bar.
Responsibilities: Support keyboard selection, safe manual entry and dismissal without navigation.
Important: Typing never reads resources; the parent routes committed choices to the owning tab.
*/
import { useEffect, useId, useRef, useState } from "react";
import { namespaceOptions } from "./namespaceOptions";

export function NamespaceControl({
  namespace,
  names,
  complete,
  disabled,
  onSelect,
}: {
  namespace: string;
  names: string[];
  complete: boolean;
  disabled: boolean;
  onSelect: (namespace: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [highlight, setHighlight] = useState(0);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const search = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);
  const id = useId();
  const options = namespaceOptions(query, names, complete, namespace);
  const active = Math.min(highlight, Math.max(0, options.length - 1));
  const expanded = open && !disabled;

  useEffect(() => {
    setOpen(false);
    setQuery("");
  }, [namespace, disabled]);
  useEffect(() => {
    if (!expanded) return;
    search.current?.focus();
    const dismiss = (event: PointerEvent) => {
      if (event.target instanceof Node && !root.current?.contains(event.target)) setOpen(false);
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, [expanded]);
  useEffect(() => {
    if (expanded) list.current?.children[active]?.scrollIntoView({ block: "nearest" });
  }, [active, expanded, query]);

  function choose(value: string) {
    setOpen(false);
    trigger.current?.focus();
    if (value !== namespace) onSelect(value);
  }
  return (
    <div
      className="namespace-control"
      ref={root}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <span id={`${id}-label`}>Namespace</span>
      <button
        ref={trigger}
        className="namespace-trigger"
        aria-label={`Namespace: ${namespace}`}
        aria-haspopup="listbox"
        aria-expanded={expanded}
        aria-controls={expanded ? `${id}-options` : undefined}
        disabled={disabled}
        onClick={() => {
          setQuery("");
          setHighlight(0);
          setOpen(!open);
        }}
        onKeyDown={(event) => {
          if (event.key === "ArrowDown" || event.key === "ArrowUp") {
            event.preventDefault();
            setQuery("");
            setHighlight(0);
            setOpen(true);
          }
        }}
      >
        <span>{namespace}</span>
        <span aria-hidden="true">⌄</span>
      </button>
      {expanded && (
        <div className="namespace-popover">
          <input
            spellCheck={false}
            autoCorrect="off"
            autoCapitalize="none"
            autoComplete="off"
            ref={search}
            role="combobox"
            aria-label="Search namespaces"
            aria-expanded="true"
            aria-controls={`${id}-options`}
            aria-autocomplete="list"
            aria-activedescendant={options.length ? `${id}-option-${active}` : undefined}
            placeholder="Search namespaces…"
            value={query}
            onChange={(event) => {
              setQuery(event.target.value);
              setHighlight(0);
            }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing) return;
              if (event.key === "Escape") {
                event.preventDefault();
                event.stopPropagation();
                setOpen(false);
                trigger.current?.focus();
              } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
                event.preventDefault();
                setHighlight(Math.max(0, Math.min(options.length - 1, active + (event.key === "ArrowDown" ? 1 : -1))));
              } else if (event.key === "Enter") {
                event.preventDefault();
                if (options[active]) choose(options[active].value);
              }
            }}
          />
          <div ref={list} id={`${id}-options`} role="listbox" aria-labelledby={`${id}-label`} className="namespace-options">
            {options.map((option, index) => (
              <div
                key={option.value}
                id={`${id}-option-${index}`}
                role="option"
                aria-selected={option.value === namespace}
                className={index === active ? "highlighted" : ""}
                onPointerMove={() => setHighlight(index)}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => choose(option.value)}
              >
                <span>{option.manual ? `Use “${option.value}”` : option.value}</span>
                {option.value === namespace && <span aria-hidden="true">✓</span>}
              </div>
            ))}
          </div>
          {!options.length && <p role="status">No matching namespaces.</p>}
          <p>{complete ? "Type to filter namespaces" : "Discovery incomplete — type a namespace to use it."}</p>
        </div>
      )}
    </div>
  );
}

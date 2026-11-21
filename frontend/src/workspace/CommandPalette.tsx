/*
Purpose: Offer discoverable target-scoped commands.
Used by: Workspace.
Responsibilities: Show shortcuts and explain disabled actions.
Important: Palette invokes the same callbacks as resource buttons.
*/
import { commands, type CommandId } from "./commands";
import { Modal } from "../ui/ReviewDialog";
export function CommandPalette({
  projects,
  available,
  onCommand,
  onClose,
}: {
  projects?: () => void;
  available: boolean;
  onCommand: (id: CommandId) => void;
  onClose: () => void;
}) {
  return (
    <Modal title="Workspace commands" onCancel={onClose}>
      <div className="command-list">
        {projects && (
          <button
            onClick={() => {
              onClose();
              projects();
            }}
          >
            Projects
          </button>
        )}
        {commands
          .filter((c) => c.id !== "palette")
          .map((c) => (
            <button
              key={c.id}
              disabled={c.target && !available}
              onClick={() => {
                onClose();
                onCommand(c.id);
              }}
            >
              {c.label}
              <kbd>{c.shortcut}</kbd>
            </button>
          ))}
      </div>
    </Modal>
  );
}

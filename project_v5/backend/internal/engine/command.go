package engine

// Command is a reversible mutation of a Document. Faithful port of v9's
// Command interface. execute and undo MUST be inverses for CommandHistory to
// behave correctly.
type Command interface {
	Execute(doc *Document) error
	Undo(doc *Document) error
}

// CommandHistory tracks executed commands for undo/redo. Mirrors v9's
// CommandHistory exactly, including the "redo stack cleared on new execute"
// rule.
type CommandHistory struct {
	undoStack []Command
	redoStack []Command
}

// NewCommandHistory returns an empty history.
func NewCommandHistory() *CommandHistory {
	return &CommandHistory{}
}

// Execute runs cmd against doc, appends it to undo, and clears redo. Errors
// from cmd.Execute are surfaced — the command is NOT pushed if it errored.
func (h *CommandHistory) Execute(cmd Command, doc *Document) error {
	if err := cmd.Execute(doc); err != nil {
		return err
	}
	h.undoStack = append(h.undoStack, cmd)
	h.redoStack = nil
	return nil
}

// Undo reverses the most recent command. Returns false if undo stack empty,
// true otherwise. Errors from cmd.Undo are surfaced; the command is NOT
// pushed to redo if undo errored.
func (h *CommandHistory) Undo(doc *Document) (bool, error) {
	n := len(h.undoStack)
	if n == 0 {
		return false, nil
	}
	cmd := h.undoStack[n-1]
	h.undoStack = h.undoStack[:n-1]
	if err := cmd.Undo(doc); err != nil {
		return false, err
	}
	h.redoStack = append(h.redoStack, cmd)
	return true, nil
}

// Redo replays the most recently undone command.
func (h *CommandHistory) Redo(doc *Document) (bool, error) {
	n := len(h.redoStack)
	if n == 0 {
		return false, nil
	}
	cmd := h.redoStack[n-1]
	h.redoStack = h.redoStack[:n-1]
	if err := cmd.Execute(doc); err != nil {
		return false, err
	}
	h.undoStack = append(h.undoStack, cmd)
	return true, nil
}

// CanUndo reports whether the undo stack is non-empty.
func (h *CommandHistory) CanUndo() bool { return len(h.undoStack) > 0 }

// CanRedo reports whether the redo stack is non-empty.
func (h *CommandHistory) CanRedo() bool { return len(h.redoStack) > 0 }

// Clear empties both stacks.
func (h *CommandHistory) Clear() {
	h.undoStack = nil
	h.redoStack = nil
}

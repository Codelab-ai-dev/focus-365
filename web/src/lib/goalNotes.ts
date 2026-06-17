import { apiFetch } from "./api";

export type GoalNote = {
  id: string;
  goal_id: string;
  note_date: string;
  body: string;
  created_at: string;
};

export function listGoalNotes(goalId: string): Promise<GoalNote[]> {
  return apiFetch<{ notes: GoalNote[] }>(`/api/v1/goals/${goalId}/notes`).then(
    (r) => r.notes
  );
}

export function createGoalNote(
  goalId: string,
  input: { note_date: string; body: string }
): Promise<GoalNote> {
  return apiFetch<{ note: GoalNote }>(`/api/v1/goals/${goalId}/notes`, {
    method: "POST",
    body: JSON.stringify(input),
  }).then((r) => r.note);
}

export function deleteGoalNote(goalId: string, noteId: string): Promise<void> {
  return apiFetch<void>(`/api/v1/goals/${goalId}/notes/${noteId}`, {
    method: "DELETE",
  });
}

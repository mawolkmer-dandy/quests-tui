import { LaunchProps, Toast, showHUD, showToast } from "@raycast/api";
import { questsAdd } from "./quests";

/** No-view command: capture a quest to the Questboard straight from the inline
 * argument, no form — the Things/Reminders "quick add" pattern. Use the full
 * "Add Quest" command when you want a campaign, description, type, or priority. */
export default async function QuickAddQuest(props: LaunchProps<{ arguments: { title: string } }>) {
  const title = props.arguments.title?.trim();
  if (!title) {
    await showToast({ style: Toast.Style.Failure, title: "Title is required" });
    return;
  }
  try {
    await questsAdd([title]);
    await showHUD("⬖ Quest captured");
  } catch (error) {
    await showToast({ style: Toast.Style.Failure, title: "Failed to add quest", message: String(error) });
  }
}

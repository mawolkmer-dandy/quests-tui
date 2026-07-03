import { Action, ActionPanel, Color, Form, Icon, Toast, closeMainWindow, popToRoot, showToast } from "@raycast/api";
import { useEffect, useState } from "react";
import { listCampaigns, questsAdd } from "./quests";

const INBOX = "__inbox__";

interface Values {
  title: string;
  note: string;
  campaign: string;
  type: string;
  important: boolean;
}

export default function AddQuest() {
  const [campaigns, setCampaigns] = useState<string[]>([]);
  const [titleError, setTitleError] = useState<string | undefined>();

  useEffect(() => {
    listCampaigns().then(setCampaigns);
  }, []);

  async function handleSubmit(values: Values) {
    if (!values.title.trim()) {
      setTitleError("Title is required");
      return;
    }
    const args: string[] = [];
    if (values.campaign && values.campaign !== INBOX) args.push("--to", values.campaign);
    if (values.type === "main") args.push("--main");
    if (values.important) args.push("--important");
    if (values.note.trim()) args.push("--note", values.note);
    args.push(values.title.trim());

    try {
      await questsAdd(args);
      await showToast({ style: Toast.Style.Success, title: "Quest captured" });
      await popToRoot();
      await closeMainWindow();
    } catch (error) {
      await showToast({ style: Toast.Style.Failure, title: "Failed to add quest", message: String(error) });
    }
  }

  return (
    <Form
      actions={
        <ActionPanel>
          <Action.SubmitForm title="Add Quest" onSubmit={handleSubmit} />
        </ActionPanel>
      }
    >
      <Form.TextField
        id="title"
        title="Title"
        placeholder="What needs doing?"
        error={titleError}
        onChange={() => titleError && setTitleError(undefined)}
      />
      <Form.TextArea id="note" title="Description" placeholder="Optional details…" />
      <Form.Dropdown id="campaign" title="Campaign" defaultValue={INBOX}>
        <Form.Dropdown.Item value={INBOX} title="Questboard (inbox)" icon={Icon.Tray} />
        {campaigns.map((c) => (
          <Form.Dropdown.Item key={c} value={c} title={c} icon={{ source: Icon.Folder, tintColor: Color.Yellow }} />
        ))}
      </Form.Dropdown>
      <Form.Dropdown id="type" title="Type" defaultValue="side">
        <Form.Dropdown.Item value="side" title="Side" icon={{ source: Icon.Circle, tintColor: Color.Blue }} />
        <Form.Dropdown.Item value="main" title="Main" icon={{ source: Icon.Circle, tintColor: Color.Yellow }} />
      </Form.Dropdown>
      <Form.Checkbox id="important" title="Priority" label="Important" defaultValue={false} />
    </Form>
  );
}

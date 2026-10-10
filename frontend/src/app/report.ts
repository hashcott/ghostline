import { Browser } from "@wailsio/runtime";
import { Service } from "./api";

// Opens GitHub's bug report form with the version, the OS and the package
// filled in by the backend.
export async function openIssueForm(): Promise<void> {
  const url = await Service.ReportIssueURL();
  await Browser.OpenURL(url);
}

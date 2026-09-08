import { afterEach, describe, expect, it, vi } from "vitest";
import { exportCSV } from "./csv";

afterEach(() => vi.restoreAllMocks());

async function download(headers: string[], rows: (string | number)[][]) {
  let exported: Blob | undefined;
  vi.spyOn(URL, "createObjectURL").mockImplementation((blob) => {
    exported = blob as Blob;
    return "blob:csv-export";
  });
  vi.spyOn(URL, "revokeObjectURL").mockImplementation(() => {});
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  exportCSV("security-events.csv", headers, rows);
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(exported!);
  });
}

describe("spreadsheet-safe CSV downloads", () => {
  it.each(["=1+1", "+1+1", "-1+1", "@SUM(1)", "＝1+1", "＋1+1", "－1+1", "＠SUM(1)", " \t=1+1", "\t=1+1", "\r=1+1", "\n=1+1"])(
    "exports an untrusted formula as text: %j",
    async (payload) => {
      const csv = await download(["user_agent"], [[payload]]);
      expect(csv).toBe(`user_agent\r\n"'${payload}"`);
    },
  );

  it("neutralizes formula-like headers as well as row values", async () => {
    expect(await download(["=1+1"], [["safe"]])).toBe('"\'=1+1"\r\nsafe');
  });

  it("keeps carriage returns and delimiter injection inside their original cell", async () => {
    expect(await download(["user_agent", "count"], [['browser\r=1+1,"payload"', 2]]))
      .toBe('user_agent,count\r\n"browser\r=1+1,""payload""",2');
    expect(await download(["user_agent"], [["browser\r=1+1"]]))
      .toBe('user_agent\r\n"browser\r=1+1"');
  });

  it("preserves ordinary text, Turkish characters, empty cells, and numeric values", async () => {
    expect(await download(["name", "count", "note"], [["Çağrı", -2, ""], ['a,"b"\nc', 0, "normal"]]))
      .toBe('name,count,note\r\nÇağrı,-2,\r\n"a,""b""\nc",0,normal');
  });
});

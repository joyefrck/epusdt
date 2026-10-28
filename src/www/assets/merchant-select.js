// Small source module shared by the existing prebuilt admin pages.
import { a as api } from "./auth-store-uaDKRR4l.js";
import { wm as jsxRuntime } from "./messages-Cl9iE86Z.js";
import { a as SelectValue, i as SelectTrigger, n as SelectContent, r as SelectItem, t as Select } from "./select-DT0a9Hyj.js";
const { jsx, jsxs } = jsxRuntime();

export function merchantLabel(wallet) {
  if (!wallet.api_key_id) return "未绑定";
  const name = wallet.merchant_name || `商户 #${wallet.api_key_id}`;
  return wallet.merchant_deleted ? `${name}（已删除）` : name;
}

export function MerchantSelect({ value, onChange, filter = false, allowUnbound = false, className = "w-full" }) {
  const query = api.useQuery("get", "/admin/api/v1/api-keys", {
    params: { query: { include_deleted: filter ? "true" : "false" } },
  }, { staleTime: 0, refetchOnMount: "always" });
  const rows = (query.data?.data ?? []).filter(row => filter || (row.status === 1 && !row.deleted_at) || String(row.id) === value);
  const options = [];
  if (filter) options.push({ id: "__all__", label: "全部商户" });
  if (filter || allowUnbound) options.push({ id: "0", label: filter ? "未归属商户" : "未绑定" });
  for (const row of rows) options.push({
    id: String(row.id),
    label: `${row.name || "未命名商户"}（PID ${row.pid}）${row.deleted_at ? " · 已删除" : row.status !== 1 ? " · 已禁用" : ""}`,
  });
  if (value && value !== "__all__" && !options.some(row => row.id === value)) {
    options.push({ id: value, label: `商户 #${value}（不可用）` });
  }
  return jsxs(Select, { value: value || "", onValueChange: onChange, children: [
    jsx(SelectTrigger, { className, style: filter ? { width: 180, maxWidth: "100%", minWidth: 0 } : undefined, "aria-label": "商户", children: jsx(SelectValue, { placeholder: query.isLoading ? "加载商户…" : "选择商户" }) }),
    jsx(SelectContent, { children: options.map(row => jsx(SelectItem, { value: row.id, children: row.label }, row.id)) }),
  ] });
}

export function merchantQuery(value) {
  return value && value !== "__all__" ? { api_key_id: Number(value) } : {};
}

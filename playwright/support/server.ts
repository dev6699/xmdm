export type DashboardServerConfig = Readonly<{
  baseURL: string;
  usesExternalServer: boolean;
}>;

export function dashboardServerConfig(): DashboardServerConfig {
  return {
    baseURL: process.env.XMDM_DASHBOARD_URL ?? 'http://127.0.0.1:39092',
    usesExternalServer: Boolean(process.env.XMDM_DASHBOARD_URL),
  };
}

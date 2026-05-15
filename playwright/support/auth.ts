export type DashboardCredentials = Readonly<{
  username: string;
  password: string;
}>;

export function dashboardCredentials(): DashboardCredentials {
  return {
    username: process.env.XMDM_DASHBOARD_USERNAME ?? 'admin',
    password: process.env.XMDM_DASHBOARD_PASSWORD ?? 'admin',
  };
}

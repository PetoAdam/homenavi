// Centralized breakpoints for responsive logic
export const BREAKPOINTS = {
  permanentSidebar: 1400,
};

export function isPermanentSidebarWidth(width) {
  return width >= BREAKPOINTS.permanentSidebar;
}

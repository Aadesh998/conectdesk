// hasFeature reports whether the installation has an item's `feature`, e.g.
// 'tenancy' for pages that only exist with multi-tenancy.
export const filterNavItems = (navItems, can, hasFeature = () => true) => {
    return navItems
        .map(item => {
            // Process children first
            const filteredChildren = item.children
                ? filterNavItems(item.children, can, hasFeature)
                : undefined
            // Check item's permission and feature
            const hasAccess = (item.permission ? can(item.permission) : true) &&
                (item.feature ? hasFeature(item.feature) : true)
            // Only keep the item if:
            // 1. Has required permission (or none required)
            // 2. Has valid children (if parent item)
            const keep = hasAccess && (!item.children || filteredChildren.length > 0)
            return keep ? { ...item, children: filteredChildren } : null
        })
        .filter(Boolean) // Remove null entries
}
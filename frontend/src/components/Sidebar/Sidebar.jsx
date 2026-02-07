import React, { forwardRef, useCallback, useEffect, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faBolt, faHouse, faLightbulb, faMap, faPlug, faStar, faUsers, faMusic } from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import './Sidebar.css';
import { useAuth } from '../../context/AuthContext';

import { getIntegrationRegistry } from '../../services/integrationService';

const FA_ICON_MAP = {
	spotify: faSpotify,
	music: faMusic,
	sparkles: faStar,
	plug: faPlug,
};

function normalizeIconKey(iconName) {
	const raw = (iconName || '').toString().trim();
	if (!raw) return '';
	return raw.toLowerCase();
}

function isBundledIntegrationIconPath(iconName) {
	const raw = (iconName || '').toString().trim();
	if (!raw) return false;
	// Only allow same-origin, bundled assets served through the integration proxy.
	return raw.startsWith('/integrations/');
}

function resolveFaIcon(iconName) {
	const key = normalizeIconKey(iconName);
	if (!key) return null;

	// Allow explicit font-awesome tokens like: "fa:plug" or "fa:spotify".
	const faKey = key.startsWith('fa:') ? key.slice('fa:'.length).trim() : key;
	return FA_ICON_MAP[faKey] || null;
}

function IntegrationSidebarIcon({ icon, fallbackKey }) {
	const [imgFailed, setImgFailed] = useState(false);

	const fa = resolveFaIcon(icon) || resolveFaIcon(fallbackKey) || faPlug;
	const raw = (icon || '').toString().trim();

	if (!imgFailed && isBundledIntegrationIconPath(raw)) {
		return (
			<img
				className="sidebar-icon-img"
				src={raw}
				alt=""
				aria-hidden="true"
				loading="lazy"
				onError={() => setImgFailed(true)}
			/>
		);
	}

	return <FontAwesomeIcon icon={fa} />;
}

const MAIN_GROUP = {
	header: 'Main',
	items: [
		{ name: 'Home', path: '/', icon: <FontAwesomeIcon icon={faHouse} /> },
		{ name: 'Devices', path: '/devices', icon: <FontAwesomeIcon icon={faLightbulb} /> },
		{ name: 'Automation', path: '/automation', icon: <FontAwesomeIcon icon={faBolt} /> },
		{ name: 'Map', path: '/map', icon: <FontAwesomeIcon icon={faMap} /> },
	],
};

const ADMIN_GROUP = {
	header: 'Admin',
	items: [
		{ name: 'Integrations', path: '/admin/integrations', icon: <FontAwesomeIcon icon={faPlug} /> },
	],
};

const Sidebar = forwardRef(function Sidebar({ menuOpen, setMenuOpen, isPermanentSidebar }, ref) {
	const location = useLocation();
	const navigate = useNavigate();
	const { user, accessToken, bootstrapping } = useAuth();
	const [integrations, setIntegrations] = useState([]);

	const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');
	const isAdmin = user && user.role === 'admin';

	const loadIntegrations = useCallback(async () => {
		if (!isResidentOrAdmin || !accessToken) {
			setIntegrations([]);
			return;
		}
		const res = await getIntegrationRegistry();
		if (res.success && Array.isArray(res.data?.integrations)) {
			setIntegrations(res.data.integrations);
		} else {
			setIntegrations([]);
		}
	}, [accessToken, isResidentOrAdmin]);

	useEffect(() => {
		let alive = true;
		if (!isResidentOrAdmin || !accessToken) {
			setIntegrations([]);
			return () => { alive = false; };
		}
		( async () => {
			await loadIntegrations();
		})();
		return () => { alive = false; };
	}, [accessToken, isResidentOrAdmin, bootstrapping, loadIntegrations]);

	useEffect(() => {
		const handler = () => {
			loadIntegrations();
		};
		window.addEventListener('homenavi:integrations-updated', handler);
		return () => {
			window.removeEventListener('homenavi:integrations-updated', handler);
		};
	}, [loadIntegrations]);

	const menuGroups = useMemo(() => {
		const integrationItems = (integrations || []).map((i) => {
			const name = i?.display_name || i?.id;
			const route = i?.id ? `/apps/${i.id}` : '/';
			const icon = <IntegrationSidebarIcon icon={i?.icon} fallbackKey={i?.id} />;
			return { name, path: route, icon };
		});

		const groups = [
			MAIN_GROUP,
			{
				header: 'Integrations',
				items: integrationItems,
			},
		];
		if (isAdmin) {
			groups.push(ADMIN_GROUP);
		}
		return groups;
	}, [integrations, isAdmin]);

	return (
		<aside
			key={isPermanentSidebar ? 'permanent' : 'overlay'}
			className={`sidebar${menuOpen ? ' open' : ''} flex flex-col justify-start`}
			aria-hidden={!menuOpen && !isPermanentSidebar}
			tabIndex={-1}
			ref={ref}
		>
			<div className="flex items-center justify-between px-2 pb-2 border-b border-white/10 mb-4">
				<button
					className="glass-logo select-none homenavi-logo-btn"
					onClick={() => navigate('/')}
					aria-label="Go to home"
					tabIndex={0}
					type="button"
				>
					homenavi
				</button>
			</div>
			<nav className="flex-1 px-1 flex flex-col gap-2" style={{ overflowY: 'auto' }}>
				{menuGroups.map(group => (
					<div className="sidebar-group" key={group.header}>
						<div className="sidebar-group-header">{group.header}</div>
						<ul className="sidebar-group-list">
							{group.items
								.filter(item => (item.name !== 'Devices' && item.name !== 'Automation' && item.name !== 'Map') || isResidentOrAdmin)
								.map(item => {
									const isActive = location.pathname === item.path || location.pathname.startsWith(`${item.path}/`);
									return (
										<li key={item.name}>
											<button
												className={`menu-item${isActive ? ' active' : ''}`}
												onClick={() => {
													navigate(item.path);
													setMenuOpen(false);
												}}
												aria-current={isActive ? 'page' : undefined}
											>
												<span className="menu-icon">{item.icon}</span>
												<span className="menu-label">{item.name}</span>
											</button>
										</li>
									);
								})}

							{isResidentOrAdmin && group.header === 'Main' && (
									<li key="Users">
										<button
											className={`menu-item${location.pathname === '/users' ? ' active' : ''}`}
											onClick={() => { navigate('/users'); setMenuOpen(false); }}
											aria-current={location.pathname === '/users' ? 'page' : undefined}
										>
											<span className="menu-icon"><FontAwesomeIcon icon={faUsers} /></span>
											<span className="menu-label">Users</span>
										</button>
									</li>
								)}
						</ul>
					</div>
				))}
			</nav>
		</aside>
	);
});

export default Sidebar;

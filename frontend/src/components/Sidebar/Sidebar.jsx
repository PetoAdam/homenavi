import React, { forwardRef } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { faHouse, faLightbulb, faMap, faUsers } from '@fortawesome/free-solid-svg-icons';
import { faSpotify } from '@fortawesome/free-brands-svg-icons';
import './Sidebar.css';
import { useAuth } from '../../context/AuthContext';

const menuGroups = [
	{
		header: 'Main',
		items: [
			{ name: 'Home', path: '/', icon: <FontAwesomeIcon icon={faHouse} /> },
			{ name: 'Devices', path: '/devices', icon: <FontAwesomeIcon icon={faLightbulb} /> },
			{ name: 'Map', path: '/map', icon: <FontAwesomeIcon icon={faMap} /> },
		],
	},
	{
		header: 'Integrations',
		items: [
			{ name: 'Spotify', path: '/spotify', icon: <FontAwesomeIcon icon={faSpotify} /> },
		],
	},
];

const Sidebar = forwardRef(function Sidebar({ menuOpen, setMenuOpen, isPermanentSidebar }, ref) {
	const location = useLocation();
	const navigate = useNavigate();
	const { user } = useAuth();

	const isResidentOrAdmin = user && (user.role === 'resident' || user.role === 'admin');

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
								.filter(item => item.name !== 'Devices' || isResidentOrAdmin)
								.map(item => (
									<li key={item.name}>
										<button
											className={`menu-item${location.pathname === item.path ? ' active' : ''}`}
											onClick={() => {
												navigate(item.path);
												setMenuOpen(false);
											}}
											aria-current={location.pathname === item.path ? 'page' : undefined}
										>
											<span className="menu-icon">{item.icon}</span>
											<span className="menu-label">{item.name}</span>
										</button>
									</li>
								))}

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

'use strict';
'require baseclass';

function lighthouse() {
	var ns = 'http://www.w3.org/2000/svg';
	var svg = document.createElementNS(ns, 'svg');
	svg.setAttribute('viewBox', '0 0 48 48');
	svg.setAttribute('class', 'fw-lighthouse');
	svg.setAttribute('aria-hidden', 'true');
	[
		['fw-beam', 'M19 14.375 L4 9.5 V22.5 L19 17.625 Z M29 14.375 L44 9.5 V22.5 L29 17.625 Z'],
		['fw-tower', 'M16.5 13 L24 6.5 L31.5 13 Z M19.5 21.5 H28.5 L31 40 H17 Z M17 19 H31 V21.5 H17 Z'],
		['fw-lamp', 'M19 13 H29 V19 H19 Z'],
		['fw-stripe', 'M18.55 28.5 H29.45 L29.85 31.5 H18.15 Z'],
		['fw-sea', 'M7 43.5 C10 41.5 13 41.5 16 43.5 S22 45.5 25 43.5 S31 41.5 34 43.5 S38.5 45 41 43.5']
	].forEach(function(part) {
		var path = document.createElementNS(ns, 'path');
		path.setAttribute('class', part[0]);
		path.setAttribute('d', part[1]);
		svg.appendChild(path);
	});
	return svg;
}

// Small stroke icons from the design component board; status text carries the meaning.
function icon(name) {
	var circle = 'M21 12a9 9 0 1 1-18 0a9 9 0 1 1 18 0';
	var paths = {
		check: circle + ' M8 12.5l2.7 2.7L16 9.5',
		ban: circle + ' M5.7 5.7l12.6 12.6',
		moon: 'M19.5 14.2A8 8 0 0 1 9.8 4.5a8 8 0 1 0 9.7 9.7z',
		info: circle + ' M12 11v5M12 8h.01',
		mast: 'M12 2.5v17M12 4.5l6.5 10H12M4 19.5c2.7 1.6 5.3 1.6 8 0s5.3-1.6 8 0',
		hand: 'M8 13V6.5a1.5 1.5 0 0 1 3 0V11M11 10V4.5a1.5 1.5 0 0 1 3 0V11M14 10.5V6a1.5 1.5 0 0 1 3 0v7c0 4-2.5 7-6 7-2.5 0-4-1.2-5.5-3.5L4 14.5a1.5 1.5 0 0 1 2.4-1.8L8 14.5'
	};
	var svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
	svg.setAttribute('class', 'fw-icon');
	svg.setAttribute('viewBox', '0 0 24 24');
	svg.setAttribute('data-icon', name);
	svg.setAttribute('aria-hidden', 'true');
	svg.setAttribute('focusable', 'false');
	var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
	path.setAttribute('d', paths[name]);
	svg.appendChild(path);
	return svg;
}

return baseclass.extend({
	lighthouse: lighthouse,
	icon: icon,

	loadStyles: function(name) {
		var id = 'fw-' + name + '-css';
		if (!document.getElementById(id)) document.head.appendChild(E('link', {
			'id': id, 'rel': 'stylesheet', 'href': L.resource('familywarden/' + name + '.css')
		}));
	},

	header: function(title, description, actions) {
		return E('div', { 'class': 'fw-head' }, [
			E('div', {}, [
				E('p', { 'class': 'fw-brand' }, [lighthouse(), 'FamilyWarden']),
				E('h2', { 'class': 'fw-title' }, title),
				E('p', { 'class': 'fw-lede' }, description)
			]),
			actions ? E('div', { 'class': 'fw-head-actions' }, actions) : ''
		]);
	}
});

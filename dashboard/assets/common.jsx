// @flow

import {faHome, faLink, faGlobeEurope, faTachometerAlt, faList} from '@fortawesome/free-solid-svg-icons';
import {faCreditCard} from '@fortawesome/free-regular-svg-icons';

type ProvidedMenuProp = {|title: string, icon: string|};
const menuSkeletons: Array<{|id: string, menu: ProvidedMenuProp|}> = [
	{
		id:   'home',
		menu: {
			title: 'Home',
			icon:  faHome,
		},
	}, {
		id:   'chain',
		menu: {
			title: 'Chain',
			icon:  faLink,
		},
	}, {
		id:   'txpool',
		menu: {
			title: 'TxPool',
			icon:  faCreditCard,
		},
	}, {
		id:   'network',
		menu: {
			title: 'Network',
			icon:  faGlobeEurope,
		},
	}, {
		id:   'system',
		menu: {
			title: 'System',
			icon:  faTachometerAlt,
		},
	}, {
		id:   'logs',
		menu: {
			title: 'Logs',
			icon:  faList,
		},
	},
];
export type MenuProp = {|...ProvidedMenuProp, id: string|};
// The sidebar menu and the main content are rendered based on these elements.
// Using the id is circumstantial in some cases, so it is better to insert it also as a value.
// This way the mistyping is prevented.
export const MENU: Map<string, {...MenuProp}> = new Map(menuSkeletons.map(({id, menu}) => ([id, {id, ...menu}])));

export const DURATION = 200;

export const chartStrokeWidth = 0.2;

export const styles = {
	light: {
		color: 'rgba(255, 255, 255, 0.54)',
	},
};

// unit contains the units for the bytePlotter.
export const unit = ['', 'Ki', 'Mi', 'Gi', 'Ti', 'Pi', 'Ei', 'Zi', 'Yi'];

// simplifyBytes returns the simplified version of the given value followed by the unit.
export const simplifyBytes = (x: number) => {
	let i = 0;
	for (; x > 1024 && i < 8; i++) {
		x /= 1024;
	}
	return x.toFixed(2).toString().concat(' ', unit[i], 'B');
};

// hues contains predefined colors for gradient stop colors.
export const hues     = ['#00FF00', '#FFFF00', '#FF7F00', '#FF0000'];
export const hueScale = [0, 2048, 102400, 2097152];

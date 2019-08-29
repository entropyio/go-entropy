// @flow
import React from 'react';
import {render} from 'react-dom';

import MuiThemeProvider from '@material-ui/core/styles/MuiThemeProvider';
import createMuiTheme from '@material-ui/core/styles/createMuiTheme';

import Dashboard from './components/Dashboard';

const theme: Object = createMuiTheme({
	// typography: {
	// 	useNextVariants: true,
	// },
	palette: {
		type: 'dark',
	},
});
const dashboard = document.getElementById('dashboard');
if (dashboard) {
	// Renders the whole dashboard.
	render(
		<MuiThemeProvider theme={theme}>
			<Dashboard />
		</MuiThemeProvider>,
		dashboard,
	);
}

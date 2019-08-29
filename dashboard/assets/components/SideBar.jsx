// @flow


import React, {Component} from 'react';

import withStyles from '@material-ui/core/styles/withStyles';
import List from '@material-ui/core/List';
import ListItem from '@material-ui/core/ListItem';
import ListItemIcon from '@material-ui/core/ListItemIcon';
import ListItemText from '@material-ui/core/ListItemText';
import Icon from '@material-ui/core/Icon';
import Transition from 'react-transition-group/Transition';
import {FontAwesomeIcon} from '@fortawesome/react-fontawesome';

import {MENU, DURATION} from '../common';

// styles contains the constant styles of the component.
const styles = {
	menu: {
		default: {
			transition: `margin-left ${DURATION}ms`,
		},
		transition: {
			entered: {marginLeft: -200},
		},
	},
};

// themeStyles returns the styles generated from the theme for the component.
const themeStyles = theme => ({
	list: {
		background: theme.palette.grey[900],
	},
	listItem: {
		minWidth: theme.spacing.unit * 7,
	},
	icon: {
		fontSize: theme.spacing.unit * 3,
		overflow: 'unset',
	},
});

export type Props = {
	classes: Object, // injected by withStyles()
	opened: boolean,
	changeContent: string => void,
};

type State = {}

// SideBar renders the sidebar of the dashboard.
class SideBar extends Component<Props, State> {
	shouldComponentUpdate(nextProps: Readonly<Props>, nextState: Readonly<State>, nextContext: any) {
		return nextProps.opened !== this.props.opened;
	}

	// clickOn returns a click event handler function for the given menu item.
	clickOn = menu => (event) => {
		event.preventDefault();
		this.props.changeContent(menu);
	};

	// menuItems returns the menu items corresponding to the sidebar state.
	menuItems = (transitionState) => {
		const {classes} = this.props;
		const children = [];
		MENU.forEach((menu) => {
			children.push((
				<ListItem button key={menu.id} onClick={this.clickOn(menu.id)} className={classes.listItem}>
					<ListItemIcon>
						<Icon className={classes.icon}>
							<FontAwesomeIcon icon={menu.icon} />
						</Icon>
					</ListItemIcon>
					<ListItemText
						primary={menu.title}
						style={{
							...styles.menu.default,
							...styles.menu.transition[transitionState],
							padding: 0,
						}}
					/>
				</ListItem>
			));
		});
		return children;
	};

	// menu renders the list of the menu items.
	menu = (transitionState: Object) => (
		<div className={this.props.classes.list}>
			<List>
				{this.menuItems(transitionState)}
			</List>
		</div>
	);

	render() {
		return (
			<Transition mountOnEnter in={this.props.opened} timeout={{enter: DURATION}}>
				{this.menu}
			</Transition>
		);
	}
}

export default withStyles(themeStyles)(SideBar);

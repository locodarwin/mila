// The Lucide icons a project can take, a few dozen chosen by hand. This
// module is loaded only when a project shows or picks one, so the icons
// never weigh on the app's first load.
import type { Component } from 'svelte';
import Folder from '@lucide/svelte/icons/folder';
import Briefcase from '@lucide/svelte/icons/briefcase';
import House from '@lucide/svelte/icons/house';
import Book from '@lucide/svelte/icons/book';
import Code from '@lucide/svelte/icons/code';
import Rocket from '@lucide/svelte/icons/rocket';
import Star from '@lucide/svelte/icons/star';
import Heart from '@lucide/svelte/icons/heart';
import Flag from '@lucide/svelte/icons/flag';
import Target from '@lucide/svelte/icons/target';
import Lightbulb from '@lucide/svelte/icons/lightbulb';
import Music from '@lucide/svelte/icons/music';
import Camera from '@lucide/svelte/icons/camera';
import Plane from '@lucide/svelte/icons/plane';
import Car from '@lucide/svelte/icons/car';
import ShoppingCart from '@lucide/svelte/icons/shopping-cart';
import Dumbbell from '@lucide/svelte/icons/dumbbell';
import GraduationCap from '@lucide/svelte/icons/graduation-cap';
import Palette from '@lucide/svelte/icons/palette';
import Wrench from '@lucide/svelte/icons/wrench';
import Leaf from '@lucide/svelte/icons/leaf';
import Coffee from '@lucide/svelte/icons/coffee';
import Gift from '@lucide/svelte/icons/gift';
import Calendar from '@lucide/svelte/icons/calendar';
import Users from '@lucide/svelte/icons/users';
import Globe from '@lucide/svelte/icons/globe';
import Film from '@lucide/svelte/icons/film';
import Gamepad2 from '@lucide/svelte/icons/gamepad-2';
import PiggyBank from '@lucide/svelte/icons/piggy-bank';
import Bug from '@lucide/svelte/icons/bug';
import Sprout from '@lucide/svelte/icons/sprout';
import PawPrint from '@lucide/svelte/icons/paw-print';
import Utensils from '@lucide/svelte/icons/utensils';
import Baby from '@lucide/svelte/icons/baby';
import Hammer from '@lucide/svelte/icons/hammer';
import PenTool from '@lucide/svelte/icons/pen-tool';

export const PROJECT_ICONS: Record<string, Component> = {
	folder: Folder,
	briefcase: Briefcase,
	house: House,
	book: Book,
	code: Code,
	rocket: Rocket,
	star: Star,
	heart: Heart,
	flag: Flag,
	target: Target,
	lightbulb: Lightbulb,
	music: Music,
	camera: Camera,
	plane: Plane,
	car: Car,
	'shopping-cart': ShoppingCart,
	dumbbell: Dumbbell,
	'graduation-cap': GraduationCap,
	palette: Palette,
	wrench: Wrench,
	leaf: Leaf,
	coffee: Coffee,
	gift: Gift,
	calendar: Calendar,
	users: Users,
	globe: Globe,
	film: Film,
	'gamepad-2': Gamepad2,
	'piggy-bank': PiggyBank,
	bug: Bug,
	sprout: Sprout,
	'paw-print': PawPrint,
	utensils: Utensils,
	baby: Baby,
	hammer: Hammer,
	'pen-tool': PenTool
};

// @flow


export type Content = {
	general: General,
	home:    Home,
	chain:   Chain,
	txpool:  TxPool,
	network: Network,
	system:  System,
	logs:    Logs,
};

export type ChartEntries = Array<ChartEntry>;

export type ChartEntry = {
	value: number,
};

export type General = {
	version: ?string,
	commit:  ?string,
};

export type Home = {
	/* TODO (kurkomisi) */
};

export type Chain = {
	/* TODO (kurkomisi) */
};

export type TxPool = {
	/* TODO (kurkomisi) */
};

export type Network = {
	peers: Peers,
	diff:  Array<PeerEvent>
};

export type PeerEvent = {
	ip:           string,
	id:           string,
	remove:       string,
	location:     GeoLocation,
	connected:    Date,
	disconnected: Date,
	ingress:      ChartEntries,
	egress:       ChartEntries,
	activity:     string,
};

export type Peers = {
	bundles: {[string]: PeerBundle},
};

export type PeerBundle = {
	location:     GeoLocation,
	knownPeers:   {[string]: KnownPeer},
	attempts: Array<UnknownPeer>,
};

export type KnownPeer = {
	connected:    Array<Date>,
	disconnected: Array<Date>,
	ingress:      Array<ChartEntries>,
	egress:       Array<ChartEntries>,
	active:       boolean,
};

export type UnknownPeer = {
	connected:    Date,
	disconnected: Date,
};

export type GeoLocation = {
	country:   string,
	city:      string,
	latitude:  number,
	longitude: number,
};

export type System = {
	activeMemory:   ChartEntries,
	virtualMemory:  ChartEntries,
	networkIngress: ChartEntries,
	networkEgress:  ChartEntries,
	processCPU:     ChartEntries,
	systemCPU:      ChartEntries,
	diskRead:       ChartEntries,
	diskWrite:      ChartEntries,
};

export type Record = {
	t:   string,
	lvl: Object,
	msg: string,
	ctx: Array<string>
};

export type Chunk = {
	content: string,
	name:    string,
};

export type Logs = {
	chunks:        Array<Chunk>,
	endTop:        boolean,
	endBottom:     boolean,
	topChanged:    number,
	bottomChanged: number,
};

export type LogsMessage = {
	source: ?LogFile,
	chunk:  Array<Record>,
};

export type LogFile = {
	name: string,
	last: string,
};

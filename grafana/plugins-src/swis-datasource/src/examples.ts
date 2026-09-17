/**
 * Starting points offered in the query editor. Every statement here names only stock
 * 2026.2 entities and properties and is validated against the extracted schema by
 * OrionGuides' `make validate`, which reads this file.
 */
export interface SwisExample {
  label: string;
  format: 'table' | 'timeseries';
  swql: string;
}

export const EXAMPLES: SwisExample[] = [
  {
    label: 'Node count by status',
    format: 'table',
    swql: `SELECT
    s.StatusName,
    COUNT(n.NodeID) AS NodeCount
FROM Orion.Nodes n
JOIN Orion.StatusInfo s ON n.Status = s.StatusId
GROUP BY s.StatusName
ORDER BY COUNT(n.NodeID) DESC`,
  },
  {
    label: 'Nodes down, not in maintenance',
    format: 'table',
    swql: `SELECT TOP 500
    n.NodeID,
    n.Caption,
    n.IPAddress,
    n.MachineType,
    n.StatusDescription,
    n.MinutesSinceLastSync,
    n.DetailsUrl
FROM Orion.Nodes n
WHERE n.Status = 2
  AND n.UnManaged = FALSE
ORDER BY n.Caption`,
  },
  {
    label: 'CPU and memory history for one node',
    format: 'timeseries',
    swql: `SELECT TOP 10000
    c.DateTime,
    c.AvgLoad,
    c.AvgPercentMemoryUsed
FROM Orion.CPULoad c
WHERE c.NodeID = 1
  AND $__timeFilter(c.DateTime)
ORDER BY c.DateTime`,
  },
  {
    label: 'Response time and loss for one node',
    format: 'timeseries',
    swql: `SELECT TOP 10000
    r.DateTime,
    r.AvgResponseTime,
    r.PercentLoss
FROM Orion.ResponseTime r
WHERE r.NodeID = 1
  AND $__timeFilter(r.DateTime)
ORDER BY r.DateTime`,
  },
  {
    label: 'Interface traffic history, one series per interface',
    format: 'timeseries',
    swql: `SELECT TOP 10000
    t.DateTime,
    t.Interface.FullName AS Interface,
    t.InAveragebps,
    t.OutAveragebps
FROM Orion.NPM.InterfaceTraffic t
WHERE t.NodeID = 1
  AND $__timeFilter(t.DateTime)
ORDER BY t.DateTime`,
  },
  {
    label: 'Busiest interfaces right now',
    format: 'table',
    swql: `SELECT TOP 25
    i.InterfaceID,
    i.FullName,
    i.InPercentUtil,
    i.OutPercentUtil,
    i.Inbps,
    i.Outbps
FROM Orion.NPM.Interfaces i
WHERE i.UnManaged = FALSE
ORDER BY i.PercentUtil DESC`,
  },
  {
    label: 'Fullest volumes',
    format: 'table',
    swql: `SELECT TOP 25
    v.Node.Caption AS NodeCaption,
    v.Caption AS Volume,
    v.VolumePercentUsed,
    v.VolumeSize,
    v.VolumeSpaceAvailable
FROM Orion.Volumes v
WHERE v.UnManaged = FALSE
ORDER BY v.VolumePercentUsed DESC`,
  },
  {
    label: 'Active alerts',
    format: 'table',
    swql: `SELECT TOP 1000
    aa.AlertObjectID,
    ao.AlertConfigurations.Name AS AlertName,
    ao.AlertConfigurations.Severity,
    ao.EntityCaption AS TriggeringObject,
    ao.RelatedNodeCaption AS NodeName,
    aa.TriggeredDateTime,
    aa.TriggeredMessage,
    aa.Acknowledged
FROM Orion.AlertActive aa
JOIN Orion.AlertObjects ao ON aa.AlertObjectID = ao.AlertObjectID
ORDER BY ao.AlertConfigurations.Severity DESC, aa.TriggeredDateTime DESC`,
  },
  {
    label: 'Alerts triggered per hour',
    format: 'timeseries',
    swql: `SELECT
    DateTrunc('hour', h.TimeStamp) AS Hour,
    COUNT(h.AlertHistoryID) AS Triggered
FROM Orion.AlertHistory h
WHERE h.EventType = 0
  AND $__timeFilter(h.TimeStamp)
GROUP BY DateTrunc('hour', h.TimeStamp)
ORDER BY DateTrunc('hour', h.TimeStamp)`,
  },
  {
    label: 'Variable: nodes (__text / __value)',
    format: 'table',
    swql: `SELECT TOP 5000
    n.Caption AS __text,
    n.NodeID AS __value
FROM Orion.Nodes n
ORDER BY n.Caption`,
  },
];

import React, { useEffect, useMemo, useState } from 'react';
import { Avatar,  Input } from 'antd';
import {  RobotOutlined, SearchOutlined } from '@ant-design/icons';
import { AgentSkillsPanel } from '@/components/agent/AgentSkillsPanel';
import { useUIStore } from '@/store/uiStore';
import { useAgentStore } from '@/store/agentStore';
import { parseSkills, resolveAgentAvatar } from '@/components/agent/agentPresentation';
import type { Agent } from '@/types/agent';
import styles from '@/layout/AppLayout.module.css';
import skillStyles from '@/layout/SkillsAgentList.module.css';
import listStyles from '@/components/sidebar/ConversationList.module.css';

const SkillsView: React.FC = () => {
  const selectedAgentId = useUIStore((s) => s.selectedAgentId);
  const setSelectedAgent = useUIStore((s) => s.setSelectedAgent);
  const agents = useAgentStore((s) => s.agents);
  const fetchAgents = useAgentStore((s) => s.fetchAgents);
  const [query, setQuery] = useState('');

  useEffect(() => { fetchAgents().catch(() => {}); }, [fetchAgents]);

  const filtered = useMemo(() => {
    const n = query.trim().toLowerCase();
    if (!n) return agents;
    return agents.filter((agent) => {
      const platformSkills = parseSkills(agent.custom_skills);
      return agent.name.toLowerCase().includes(n)
        || platformSkills.some((skill) => skill.name.toLowerCase().includes(n));
    });
  }, [agents, query]);

  const skillData = useMemo(() =>
    filtered.map((agent) => ({
      agent,
      skillCount: parseSkills(agent.custom_skills).length,
      baseSkillCount: parseSkills(agent.capabilities_json).length,
    })),
  [filtered]);

  const handleSelectAgent = (agent: Agent) => setSelectedAgent(agent.id);
  const selectedAgent = agents.find((a) => a.id === selectedAgentId) ?? null;

  return (
    <>
      <div className={styles.convPanelHeader}>
        <span className={styles.convPanelTitle}>技能</span>
      </div>
      <div className={listStyles.list}>
        <div className={listStyles.searchWrap}>
          <Input
            prefix={<SearchOutlined />}
            placeholder="搜索技能..."
            allowClear
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className={listStyles.searchInput}
          />
        </div>
        <div className={listStyles.items}>
          <div className={skillStyles.grid}>
            {skillData.length === 0 && (
              <div className={skillStyles.empty}>{query.trim() ? '无匹配结果' : '暂无 Agent'}</div>
            )}
            {skillData.map(({ agent, skillCount, baseSkillCount }) => {
              const isSelected = agent.id === selectedAgentId;
              return (
                <button
                  key={agent.id}
                  className={`${skillStyles.card} ${isSelected ? skillStyles.cardActive : ''}`}
                  type="button"
                  onClick={() => handleSelectAgent(agent)}
                >
                  <Avatar size={36} src={resolveAgentAvatar(agent)} icon={<RobotOutlined />} className={skillStyles.avatar} />
                  <div className={skillStyles.info}>
                    <span className={skillStyles.name}>{agent.name}</span>
                    <span className={skillStyles.meta}>已分配 {skillCount} · 底座 {baseSkillCount}</span>
                  </div>
                </button>
              );
            })}
          </div>
        </div>
      </div>
      {/* 右侧面板 */}
      {selectedAgent ? (
        <AgentSkillsPanel agent={selectedAgent} />
      ) : (
        <div className={styles.skillsEmptyPanel}>
          <div className={styles.emptyRightTitle}>选择一个 Agent 管理技能</div>
          <div className={styles.emptyRightDesc}>左侧会展示每个 Agent 的已分配 Skills 和底座 Skills 数量</div>
        </div>
      )}
    </>
  );
};

export default SkillsView;
